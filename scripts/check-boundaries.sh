#!/usr/bin/env bash
#
# check-boundaries.sh -- turn Phase 4's module boundaries into a build failure
# instead of a paragraph in a plan document.
#
#   Check 1  Wire (`json:`) tags live only in a feature's http adapter.
#   Check 2  A feature's postgres adapter only queries tables it owns,
#            where "owns" is read out of db/OWNERSHIP.md at run time.
#   Check 3  No feature imports another feature's postgres/http adapter.
#
# Run via `make check-boundaries`. Exits 0 and prints "Boundaries OK" when
# clean; on failure it prints every violation as file:line and exits 1.
#
# Written for bash 3.2 (macOS system bash) as well as CI's bash 5: no
# associative arrays, no `mapfile`. Per-feature data is expressed as `case`
# blocks so that changing it is a visible diff.

set -euo pipefail

cd "$(cd "$(dirname "$0")/.." && pwd)"

# Deterministic sort/grep collation regardless of the caller's locale.
export LC_ALL=C

VIOLATIONS="$(mktemp)"
trap 'rm -f "$VIOLATIONS"' EXIT

report() { printf '%s\n' "$*" >>"$VIOLATIONS"; }

# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------

# Directories under internal/ that are not feature modules. They are either
# shared infrastructure or the wiring layer, and both checks below treat them
# differently from features.
NON_FEATURE_DIRS='apperror bootstrap config platform testhelper transport'

is_non_feature() {
	case " $NON_FEATURE_DIRS " in
	*" $1 "*) return 0 ;;
	esac
	return 1
}

# feature_dirs prints the name of every feature module, derived from the tree
# rather than hardcoded, so a new feature is covered the day it is added.
feature_dirs() {
	local dir name
	for dir in internal/*/; do
		name="$(basename "$dir")"
		is_non_feature "$name" && continue
		printf '%s\n' "$name"
	done
}

# ---------------------------------------------------------------------------
# Check 1 -- wire tags belong to the http adapter, nowhere else
# ---------------------------------------------------------------------------
#
# Phase 4 moved every feature's wire DTOs into internal/<feature>/http/. A
# `json:` tag on a domain model means the model has started doubling as a
# transport type again, which is what the phase existed to undo.
#
# Exempt by location:
#   */http/*            the wire adapters -- this is where tags belong
#   *_test.go           tests may build wire payloads inline
#   internal/config/    envconfig, not a domain model (no json tags today,
#                       but the exemption is kept so that adding one to a
#                       config struct is not mistaken for a domain leak)
#   internal/platform/  transport infrastructure; internal/platform/paging/
#                       {cursor,offset}.go are the shared pagination envelope
#
# Exempt by explicit path allowlist -- one entry per line. This is a variable
# rather than another anonymous `grep -v` so that adding an entry is an
# obvious, reviewable act that shows up in a diff with its justification.
#
#   internal/payment/gateway.go
#     ChargeRequest / ChargeResponse / RefundRequest / RefundResponse are the
#     *external* payment gateway's wire contract, not this system's. The tags
#     describe someone else's API, and payment/stripe + payment/midtrans
#     marshal these structs when calling out to it. An unexplained exemption
#     in a lint rule is how the rule erodes, so this one carries its reason.
JSON_TAG_ALLOWLIST='
internal/payment/gateway.go
'

is_json_tag_allowlisted() {
	printf '%s\n' "$JSON_TAG_ALLOWLIST" | grep -qxF -- "$1"
}

check_wire_tags() {
	local file line

	# 1a. json: tags on types this system owns, outside the http adapters.
	while IFS= read -r file; do
		is_json_tag_allowlisted "$file" && continue
		while IFS= read -r line; do
			report "json tag outside an http adapter: ${file}:${line%%:*}
    Wire DTOs belong in internal/<feature>/http/. Domain models carry no json tags.
    If this type really is someone else's wire contract, add it to
    JSON_TAG_ALLOWLIST in scripts/check-boundaries.sh with a reason."
		done < <(grep -n 'json:"' "$file" || true)
	done < <(find internal -type f -name '*.go' \
		! -path '*/http/*' \
		! -name '*_test.go' \
		! -path 'internal/config/*' \
		! -path 'internal/platform/*' \
		| sort)

	# 1b. json:"-" anywhere under internal/ outside an http adapter.
	#
	# Phase 4 replaced all 13 of these with omission from a DTO. The point is
	# that a field is now private *by default* rather than private by someone
	# remembering to write a tag. This must stay at zero -- no allowlist, and
	# no exemption for tests, config or platform.
	# `grep -rn` prints file:line:content; keep the first two fields.
	while IFS= read -r loc; do
		report "json:\"-\" found outside an http adapter: ${loc}
    Phase 4 replaced every json:\"-\" with omission from a DTO. A field is
    private because no DTO exposes it, not because a tag hides it."
	done < <(grep -rn 'json:"-"' --include='*.go' internal/ \
		| grep -v '/http/' \
		| cut -d: -f1,2 || true)

	# 1c. internal/<feature>/dto.go must not come back.
	#
	# These files were deleted this phase; their contents now live beside the
	# handler that serialises them in internal/<feature>/http/.
	while IFS= read -r file; do
		report "resurrected DTO file: ${file}
    internal/*/dto.go was deleted in Phase 4. Wire types live in
    internal/<feature>/http/ next to the handler that serialises them."
	done < <(find internal -mindepth 2 -maxdepth 2 -type f -name 'dto.go' | sort)
}

# ---------------------------------------------------------------------------
# Check 2 -- a feature's postgres adapter only queries tables it owns
# ---------------------------------------------------------------------------
#
# ARCHITECTURE.md section 6 ("Modules own their data"): "A module's SQL may
# only name tables it owns. Cross-module reads go through a port." Go-level
# boundaries are worthless if cart reaches into products in SQL anyway.
#
# The ownership map is NOT in this file. It is parsed out of db/OWNERSHIP.md
# every run, so the document a human reads and the list CI enforces cannot
# drift apart -- there is only one list. A doc that merely agreed with a shell
# array today would disagree with it in six months, and then the doc is a lie
# and the check is the only truth. See the parsing contract in that document.
#
# Notes on the extraction:
#   - Go `//` and SQL `--` line comments are stripped before matching. English
#     prose in comments ("picked up from...", "assembles the...", "derived
#     from the limit") otherwise reads as a table reference.
#   - CTE names declared in the same file are collected and treated as
#     legitimate, because they are not tables. Three exist today:
#     `ancestors` (category, recursive) and `picked` (notification, payment).
#   - `FROM (` opens a subquery or a VALUES list, not a table, so it is
#     skipped: the identifier pattern requires a letter or underscore.
#   - `FOR UPDATE [SKIP LOCKED]` and `ON CONFLICT ... DO UPDATE SET` are the two
#     places where `UPDATE` is not followed by a table name. Both are removed by
#     phrase before extraction rather than by adding `set` and `skip` to a
#     skip-list of names: a name-based skip-list is somewhere a real table can
#     hide, and a phrase-based one is not.
#   - Only non-test files are scanned. Test files legitimately seed and assert
#     against sibling tables to satisfy foreign keys; that is fixture setup,
#     not an architectural crossing. This also removes the last source of
#     prose false positives, which lived in test names ("removes all items
#     from cart", "returns top products from paid orders").
#   - `pg_constraint`, a Postgres catalog table rather than a domain table,
#     appears only in internal/cart/postgres/repository_test.go, which asserts
#     each foreign key's ON DELETE action. Because tests are out of scope it
#     needs no allowlist entry -- recorded here so the next reader does not
#     rediscover it as a violation.
#
# `dashboard` is exempt from this check entirely. That is a deliberate
# architectural decision, not an oversight. ARCHITECTURE.md section 6 states
# the carve-out directly: "dashboard is a reporting read-model and may
# read-only join across anything. Expressing a revenue aggregate as
# cross-module service calls instead of a GROUP BY would be slower *and* less
# correct."
CHECK_2_EXEMPT_FEATURES='dashboard'

# The single source of truth for who owns what. Prose for a human, parseable by
# the awk below; the contract binding the two together is written down in the
# document, beside the table, so that whoever reformats it is looking at the
# rules while they do it.
OWNERSHIP_DOC='db/OWNERSHIP.md'

# parse_ownership_doc prints "<table> <owner>" for each row of the table between
# the BEGIN/END OWNERSHIP TABLE markers in $OWNERSHIP_DOC. Rows are split on
# `|`; backticks and surrounding whitespace are stripped; the header and `---`
# separator rows are recognised and dropped.
parse_ownership_doc() {
	awk '
		/^<!-- BEGIN OWNERSHIP TABLE -->/ { inside = 1; next }
		/^<!-- END OWNERSHIP TABLE -->/   { inside = 0; next }
		inside && /^\|/ {
			gsub(/`/, "")
			split($0, cell, "|")
			table = cell[2]
			owner = cell[3]
			gsub(/^[ \t]+|[ \t]+$/, "", table)
			gsub(/^[ \t]+|[ \t]+$/, "", owner)
			if (table == "" || owner == "") next
			if (table == "Table" && owner == "Owner") next
			if (table ~ /^-+$/) next
			print table, owner
		}
	' "$OWNERSHIP_DOC"
}

# Parsed once, up front: every later lookup reads this variable, so a malformed
# document produces one clear failure instead of twelve confusing ones.
if [ -f "$OWNERSHIP_DOC" ]; then
	OWNERSHIP_ROWS="$(parse_ownership_doc)"
else
	OWNERSHIP_ROWS=''
	report "$OWNERSHIP_DOC is missing
    Check 2 reads the table→module map from that file. Without it there is no
    ownership to enforce."
fi

# migration_tables prints every table db/migrations/ creates. Only each file's
# `-- +goose Up` section is read: a Down section's DROP TABLE list mirrors the
# Up's CREATE TABLE list, and counting both would make every table look like it
# had been created twice. Table names are matched literally in uppercase,
# because every migration writes `CREATE TABLE IF NOT EXISTS <name>`.
migration_tables() {
	awk '
		/^-- \+goose Up/   { section = "up";   next }
		/^-- \+goose Down/ { section = "down"; next }
		section == "up" && /CREATE TABLE/ {
			if (match($0, /CREATE TABLE[[:space:]]+(IF NOT EXISTS[[:space:]]+)?[a-z_][a-z0-9_]*/)) {
				name = substr($0, RSTART, RLENGTH)
				sub(/^CREATE TABLE[[:space:]]+(IF NOT EXISTS[[:space:]]+)?/, "", name)
				print name
			}
		}
	' db/migrations/*.sql | sort -u
}

# allowed_tables prints the tables a feature's postgres adapter may name,
# straight out of $OWNERSHIP_DOC. An empty result means the feature has no row
# in the document, which is itself a failure: a new postgres adapter must
# declare what it owns somewhere a human will read it.
allowed_tables() {
	printf '%s\n' "$OWNERSHIP_ROWS" | awk -v want="$1" '$2 == want { printf "%s ", $1 }'
}

# check_ownership_doc validates the document itself, before anything trusts it.
# An ownership map that is silently empty, self-contradictory, or out of step
# with the schema would let check 2 pass vacuously, which is the failure mode
# this whole arrangement exists to avoid.
check_ownership_doc() {
	local dupes only_doc only_schema

	[ -f "$OWNERSHIP_DOC" ] || return 0

	if [ -z "$OWNERSHIP_ROWS" ]; then
		report "no ownership rows parsed out of $OWNERSHIP_DOC
    Check 2 reads the rows between the BEGIN/END OWNERSHIP TABLE markers.
    See the parsing contract in that document; it is stated beside the table."
		return 0
	fi

	# Two rows for one table means two owners, which the model does not allow.
	# Catching it here beats letting whichever row awk saw last decide.
	dupes="$(printf '%s\n' "$OWNERSHIP_ROWS" | awk '{print $1}' | sort | uniq -d | tr '\n' ' ')"
	if [ -n "${dupes// /}" ]; then
		report "table listed more than once in $OWNERSHIP_DOC: ${dupes% }
    Each table has exactly one owning module, so exactly one row."
	fi

	# Drift against the schema, in both directions. A row for a table that no
	# longer exists quietly widens its owner's allowlist; a table with no row
	# narrows everyone's, including its owner's, so its own queries fail.
	only_doc="$(comm -23 \
		<(printf '%s\n' "$OWNERSHIP_ROWS" | awk '{print $1}' | sort -u) \
		<(migration_tables) | tr '\n' ' ')"
	if [ -n "${only_doc// /}" ]; then
		report "$OWNERSHIP_DOC records a table no migration creates: ${only_doc% }
    Either db/migrations/ dropped or renamed it, or the row is a typo. A stale
    row silently widens what its owner is allowed to name."
	fi

	only_schema="$(comm -13 \
		<(printf '%s\n' "$OWNERSHIP_ROWS" | awk '{print $1}' | sort -u) \
		<(migration_tables) | tr '\n' ' ')"
	if [ -n "${only_schema// /}" ]; then
		report "table created by a migration with no owner in $OWNERSHIP_DOC: ${only_schema% }
    Every table has exactly one owning module. Add a row saying which."
	fi
}

# strip_comments removes Go `//` and SQL `--` line comments, and lowercases
# the result so every later pattern can be written case-sensitively (bash 3.2
# and BSD tooling disagree about case-insensitive flags often enough to avoid
# relying on them).
strip_comments() {
	sed -e 's://.*::' -e 's/--.*//' "$1" | tr '[:upper:]' '[:lower:]'
}

# sql_table_refs prints the identifier following each FROM / JOIN / INSERT INTO
# / UPDATE. Handles LEFT/INNER/CROSS JOIN and `JOIN x ON ...` for free, because
# it anchors on the JOIN keyword itself rather than trying to parse the clause.
#
# Writes count, not just reads. `INSERT INTO another_module_table` is a worse
# violation than a join, and matching only FROM/JOIN would let it through.
#
# `for update` and `do update` are deleted first. They are the only two places
# where `update` is followed by something other than a table (`FOR UPDATE SKIP
# LOCKED`, `ON CONFLICT ... DO UPDATE SET`), and removing the phrase is safer
# than excusing the words `skip` and `set` by name -- a name-based skip-list is
# a place a real table could hide. The cost is that `FOR UPDATE OF <table>`
# would go unseen, which is fine: it is a lock hint on a table the same query
# has already named.
#
# That deletion spells its word boundaries as `(^|[^a-z0-9_])` and
# `([^a-z0-9_]|$)` rather than `\b`. BSD sed (macOS) does not implement `\b` and
# does not complain about it either -- it just matches nothing, which would
# quietly restore `set` and `skip` as phantom table names on one platform only.
# `grep -E` below is fine with `\b` on both platforms; sed is not.
#
# The trailing `|| true` matters: under `set -o pipefail` a grep that matches
# nothing (a file with no SQL, or no CTEs) fails the whole pipeline, and an
# assignment such as `x=$(sql_cte_names f)` would then trip `set -e`.
sql_table_refs() {
	strip_comments "$1" \
		| sed -E -e 's/(^|[^a-z0-9_])(for|do)[[:space:]]+update([^a-z0-9_]|$)/\1 \3/g' \
		| grep -oE '\b(from|join|into|update)[[:space:]]+[a-z_][a-z0-9_]*' \
		| awk '{print $2}' \
		| sort -u || true
}

# sql_cte_names prints CTE names declared in the file: `WITH <name> AS (`,
# `WITH RECURSIVE <name> AS (`, and `, <name> AS (` for chained CTEs.
sql_cte_names() {
	strip_comments "$1" \
		| grep -oE '(\bwith[[:space:]]+(recursive[[:space:]]+)?|,[[:space:]]*)[a-z_][a-z0-9_]*[[:space:]]+as[[:space:]]*\(' \
		| sed -E 's/[[:space:]]+as[[:space:]]*\($//' \
		| sed -E 's/^(with[[:space:]]+(recursive[[:space:]]+)?|,[[:space:]]*)//' \
		| sort -u || true
}

check_table_ownership() {
	local feature allowed file ref legit found

	# With no ownership data there is nothing to check, and looping anyway would
	# bury check_ownership_doc's one accurate diagnosis under a "feature X owns
	# no table" line for every module. Report the cause once; stop there.
	[ -n "$OWNERSHIP_ROWS" ] || return 0

	while IFS= read -r feature; do
		case " $CHECK_2_EXEMPT_FEATURES " in
		*" $feature "*) continue ;;
		esac
		[ -d "internal/$feature/postgres" ] || continue

		allowed="$(allowed_tables "$feature")"
		if [ -z "${allowed// /}" ]; then
			report "feature '$feature' has a postgres adapter but owns no table in $OWNERSHIP_DOC
    Add a row per table it owns. If it genuinely owns none, it is either a
    reporting read-model (see the carve-out in $OWNERSHIP_DOC) or it should not
    have a postgres adapter at all."
			continue
		fi

		for file in "internal/$feature/postgres"/*.go; do
			[ -f "$file" ] || continue
			case "$file" in *_test.go) continue ;; esac

			# CTE names are file-local and are not tables.
			legit="$allowed $(sql_cte_names "$file" | tr '\n' ' ')"

			while IFS= read -r ref; do
				[ -n "$ref" ] || continue
				found=1
				case " $legit " in
				*" $ref "*) found=0 ;;
				esac
				if [ "$found" = 1 ]; then
					report "$(printf 'cross-module table reference: %s names table %s, which %s does not own\n    %s may query: %s\n    Ownership is recorded in %s; cross-module reads go through a port\n    (see ARCHITECTURE.md section 6). Widen the document only to record a\n    decision, never to silence this message.' \
						"$file" "$ref" "$feature" "$feature" "${allowed% }" "$OWNERSHIP_DOC")"
				fi
			done < <(sql_table_refs "$file")
		done
	done < <(feature_dirs)
}

# ---------------------------------------------------------------------------
# Check 3 -- no feature reaches into another feature's adapter
# ---------------------------------------------------------------------------
#
# Features talk to each other through consumer-declared ports (for example
# internal/order/inventory.go), never by grabbing a sibling's concrete
# adapter. Importing internal/<other>/postgres or internal/<other>/http
# couples a feature to another feature's storage or transport shape.
#
# Exempt: the wiring layer. internal/bootstrap/ and internal/transport/ exist
# precisely to import adapters and wire them together; they are excluded by
# NON_FEATURE_DIRS above, which also keeps internal/platform/http and
# internal/transport/http/middleware -- shared infrastructure that happens to
# live at an `http` path -- from being mistaken for feature adapters. Test
# files are exempt too.
check_adapter_imports() {
	local module module_re feature_alt feature file target hit

	module="$(awk '/^module /{print $2; exit}' go.mod)"
	if [ -z "$module" ]; then
		report 'could not read the module path from go.mod'
		return 0
	fi
	# Escape regex metacharacters in the module path ("github.com/..." has dots).
	module_re="$(printf '%s' "$module" | sed -e 's/[.[\*^$\/]/\\&/g')"

	feature_alt="$(feature_dirs | tr '\n' '|' | sed -e 's/|$//')"
	[ -n "$feature_alt" ] || return 0

	while IFS= read -r feature; do
		while IFS= read -r file; do
			while IFS= read -r hit; do
				[ -n "$hit" ] || continue
				# hit looks like "12:<module>/internal/<target>/postgres"
				target="$(printf '%s' "$hit" | sed -E 's#^.*/internal/([^/]+)/(postgres|http)$#\1#')"
				[ "$target" = "$feature" ] && continue
				report "feature '$feature' imports another feature's adapter: ${file}:${hit%%:*}
    ${hit#*:}
    Features talk through consumer-declared ports (e.g. internal/order/inventory.go),
    not by importing a sibling's postgres/http package. Only internal/bootstrap/
    and internal/transport/ may wire adapters together."
			done < <(grep -noE "\"${module_re}/internal/(${feature_alt})/(postgres|http)\"" "$file" \
				| tr -d '"' || true)
		done < <(find "internal/$feature" -type f -name '*.go' ! -name '*_test.go' | sort)
	done < <(feature_dirs)
}

# ---------------------------------------------------------------------------

check_wire_tags
check_ownership_doc
check_table_ownership
check_adapter_imports

if [ -s "$VIOLATIONS" ]; then
	echo "Architectural boundary violations found:" >&2
	echo >&2
	cat "$VIOLATIONS" >&2
	echo >&2
	echo "See ARCHITECTURE.md and scripts/check-boundaries.sh for the rules." >&2
	exit 1
fi

echo "Boundaries OK"
