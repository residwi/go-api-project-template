#!/usr/bin/env bash
#
# check-boundaries.sh -- turn Phase 4's module boundaries into a build failure
# instead of a paragraph in a plan document.
#
#   Check 1  Wire (`json:`) tags live only in a slice's http adapter.
#   Check 2  db/OWNERSHIP.md itself has no duplicate rows, no row for a
#            table no migration creates, and no table with no owning row.
#   Check 3  A feature's SQL -- anywhere under the module, not only its
#            postgres adapter -- only queries tables it owns, where "owns"
#            is read out of db/OWNERSHIP.md at run time.
#   Check 4  A module imports only <feature>/contract from another module.
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

# Where the feature modules live. Everything under here is a feature and
# nothing outside it is, so the checks below no longer need a hand-maintained
# list of what to exclude. The previous arrangement subtracted a NON_FEATURE_DIRS
# denylist from internal/*/, and that list had already drifted once: `money` was
# missing from it, so a shared value object was being treated as a module. A
# denylist is wrong every time someone adds a directory and forgets; a directory
# is right by construction.
MODULES_ROOT='internal/modules'

# The directories whose entire job is to import adapters and wire them together.
# Only these are exempt from check 4 as importers. This one stays a list because
# it is a genuine permission grant, not a classification: it should be short, and
# adding to it should be a visible, argued diff.
WIRING_DIRS='bootstrap transport'

is_wiring() {
	case " $WIRING_DIRS " in
	*" $1 "*) return 0 ;;
	esac
	return 1
}

# feature_dirs prints the name of every feature module, read straight out of
# $MODULES_ROOT, so a new feature is covered the day the directory is created.
feature_dirs() {
	local dir
	for dir in "$MODULES_ROOT"/*/; do
		[ -d "$dir" ] || continue
		printf '%s\n' "$(basename "$dir")"
	done
}

# $MODULES_ROOT is asserted, not assumed. Checks 3 and 4 are both driven by
# feature_dirs, and both fail *open* when it yields nothing: check_table_ownership
# loops zero times, and check_cross_module_imports bails out on an empty feature
# alternation. Rename or empty this directory and those two report nothing at
# all, whatever is in the tree -- verified by moving internal/modules aside with
# a cross-module `INSERT INTO orders` and a sibling postgres import planted:
# neither was mentioned. (That run still exited 1, but only because check 1's
# path-keyed allowlist stopped matching the moved payment/gateway.go and
# reported 17 of its own exempt tags. Noise about the move, not the violations,
# and nothing to rely on: check 1 is location-based, so where the tree lands
# decides whether anything is said at all.) It is the failure mode check 1c's
# old -maxdepth bound had, now on the two checks that carry the most weight.
# There is no legitimately empty state here: a tree with no feature modules is a
# broken checkout or a move this script has not been told about, so it is a hard
# error rather than a violation report.
if [ -z "$(feature_dirs)" ]; then
	echo "check-boundaries: MODULES_ROOT ($MODULES_ROOT) holds no feature module directories." >&2
	echo >&2
	echo "  Checks 3 (table ownership) and 4 (cross-module imports) enumerate every feature" >&2
	echo "  out of MODULES_ROOT. With nothing there they would pass while checking" >&2
	echo "  nothing, so this is refused instead." >&2
	echo >&2
	echo "  If the modules moved, update MODULES_ROOT in scripts/check-boundaries.sh." >&2
	exit 1
fi

# importer_roots prints every path check 4 walks: everything under internal/
# that may not import another module's internals -- that is, everything except
# the wiring layer. It is a superset of feature_dirs: internal/platform must not
# import product/domain either, and "not a feature" is not the same permission
# as "may wire adapters", which is why shared infrastructure is scanned too.
#
# It prints paths rather than bare names because the two kinds of directory now
# sit at different depths: internal/platform against internal/modules/product.
# $MODULES_ROOT itself is expanded into its children rather than scanned as one
# directory, so that check 4 can tell which module an offending file belongs to:
# the caller takes the basename of what it is given, and walking the root as one
# directory would call every module's file "modules" and report each feature's
# own adapter imports as violations.
#
# That expansion leaves a hole, which is why this prints roots and not only
# directories: a .go file sitting *directly* at $MODULES_ROOT/ is in neither the
# first loop (which skips the root) nor the second (which lists only children),
# so nothing would scan it. Such a file belongs to no module -- it is not wiring
# either -- so each one is handed to the caller by name. `find` accepts a file
# argument, and the basename of foo.go can never equal a module name, so an
# adapter import in it is always reported.
importer_roots() {
	local dir name file
	for dir in internal/*/; do
		name="$(basename "$dir")"
		[ "internal/$name" = "$MODULES_ROOT" ] && continue
		is_wiring "$name" && continue
		printf '%s\n' "internal/$name"
	done
	for dir in "$MODULES_ROOT"/*/; do
		[ -d "$dir" ] || continue
		printf '%s\n' "${dir%/}"
	done
	for file in "$MODULES_ROOT"/*.go; do
		[ -f "$file" ] || continue
		printf '%s\n' "$file"
	done
}

# ---------------------------------------------------------------------------
# Check 1 -- wire tags belong to the http adapter, nowhere else
# ---------------------------------------------------------------------------
#
# Phase 4 moved every feature's wire DTOs into internal/modules/<feature>/http/.
# A `json:` tag on a domain model means the model has started doubling as a
# transport type again, which is what the phase existed to undo.
#
# Exempt by location:
#   internal/modules/<feature>/<slice>/http/*.go
#                       the wire adapters -- this is where tags belong. A
#                       feature's own root http/ (internal/modules/<feature>/http/)
#                       is one directory level short of this and is deliberately
#                       NOT exempt: that directory holds routes.go -- RouteDeps
#                       and RegisterRoutes only, per AGENTS.md -- so a json tag
#                       appearing there means a DTO has drifted into the route
#                       table instead of the slice that owns it. Likewise
#                       internal/transport/http/ (the top-level router) is not
#                       exempt: it wires slices together and defines no wire
#                       types of its own. `is_slice_http` below is the one
#                       predicate for "is this path the exempt location", used
#                       by both the walk and its filter, so the two cannot drift
#                       apart the way a glob and a prose description can.
#   *_test.go           tests may build wire payloads inline
#   internal/platform/  transport infrastructure; internal/platform/config/ is
#                       envconfig, not a domain model (no json tags today, but
#                       the exemption matters so that adding one to a config
#                       struct is not mistaken for a domain leak);
#                       internal/platform/paging/{cursor,offset}.go are the
#                       shared pagination envelope
#
# Exempt by explicit path allowlist -- one entry per line. This is a variable
# rather than another anonymous `grep -v` so that adding an entry is an
# obvious, reviewable act that shows up in a diff with its justification.
#
#   internal/modules/payment/gateway/gateway.go
#     ChargeRequest / ChargeResponse / RefundRequest / RefundResponse are the
#     *external* payment gateway's wire contract, not this system's. The tags
#     describe someone else's API. Only this one file needs the exemption --
#     gateway/stripe, gateway/midtrans and gateway/mock call out to it with
#     these same structs but declare none of their own, so allowlisting the
#     whole gateway/ directory would exempt three files that carry zero json
#     tags today, on the strength of one that does. This was a single file
#     at the module root (payment/gateway.go) before payment was sliced into
#     vertical slices; moving it under payment/gateway/ still narrows the
#     exemption, because a json tag on a domain type in payment's own model
#     can no longer land in the same file as this one by accident -- but the
#     allowlist entry itself stays exactly as wide as the one file that
#     earns it. An unexplained exemption in a lint rule is how the rule
#     erodes, so this one carries its reason.
#
#   internal/transport/http/response/response.go
#     Response and Error are the shared envelope every handler in every
#     slice writes through -- the same role internal/platform/paging's
#     cursor/offset envelope plays, just one layer up in the wiring tier
#     instead of platform. It used to be exempt for free, as a side effect
#     of the old */http/* glob matching any path containing "/http/"
#     (transport/http/ qualified); narrowing that glob to slice http/
#     directories dropped it, so it is named here instead. The other three
#     files in this package (bind.go, error_mapper.go, pagination_cursor.go)
#     need no entry -- they carry no json tags today.
JSON_TAG_ALLOWLIST='
internal/modules/payment/gateway/gateway.go
internal/transport/http/response/response.go
'

is_json_tag_allowlisted() {
	printf '%s\n' "$JSON_TAG_ALLOWLIST" | grep -qxF -- "$1"
}

# is_slice_http is true only for a slice's own http adapter --
# internal/modules/<feature>/<slice>/http/*.go. A feature's root http/
# (internal/modules/<feature>/http/*.go) is exactly one path segment short of
# this pattern and correctly returns false: that directory holds routes.go,
# never a DTO.
is_slice_http() {
	case "$1" in
	"$MODULES_ROOT"/*/*/http/*.go) return 0 ;;
	esac
	return 1
}

check_wire_tags() {
	local file line

	# 1a. json: tags on types this system owns, outside a slice's http adapter.
	while IFS= read -r file; do
		is_slice_http "$file" && continue
		is_json_tag_allowlisted "$file" && continue
		while IFS= read -r line; do
			report "json tag outside an http adapter: ${file}:${line%%:*}
    Wire DTOs belong in internal/modules/<feature>/<slice>/http/. Domain models carry no json tags.
    If this type really is someone else's wire contract, add it to
    JSON_TAG_ALLOWLIST in scripts/check-boundaries.sh with a reason."
		done < <(grep -n 'json:"' "$file" || true)
	done < <(find internal -type f -name '*.go' \
		! -name '*_test.go' \
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

	# 1c. A feature's dto.go must not come back.
	#
	# These files were deleted in Phase 4; their contents now live beside the
	# handler that serialises them in internal/modules/<feature>/http/.
	#
	# No -mindepth/-maxdepth. They used to pin this to internal/<feature>/dto.go
	# at exactly depth 2, and Phase 6 moved the features to depth 3 -- a depth
	# bound would have kept exiting 0 while checking nothing, which is the one
	# failure mode of a lint rule that nobody notices. Any dto.go anywhere under
	# internal/ is the thing being refused, so the check says exactly that.
	while IFS= read -r file; do
		report "resurrected DTO file: ${file}
    dto.go was deleted in Phase 4. Wire types live in
    internal/modules/<feature>/http/ next to the handler that serialises them."
	done < <(find internal -type f -name 'dto.go' | sort)
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
#   - Matching is NOT line-oriented. Comments are stripped per line and then all
#     whitespace, newlines included, is collapsed to single spaces, so a keyword
#     left at the end of a line still finds its table:
#         INSERT INTO
#             products (id, name)
#     Line-oriented matching missed that entirely, and this codebase already
#     wraps SQL mid-statement (internal/modules/inventory/postgres/repository.go ends a
#     line with `DO UPDATE`), so a maintainer reformatting a long column list
#     could ship a cross-module write green. Collapsing is safe because the
#     pattern requires whitespace *and then* a letter or underscore after the
#     keyword: Go syntax that follows such a line -- `update(`, `from "`,
#     `into)` -- cannot form a reference. Verified by extracting refs from every
#     .go file under internal/, cmd/ and test/ with and without the collapse:
#     zero differences.
#   - CTE names are collected and exempted, because a CTE is not a table -- but
#     ONLY when the name is not itself a table in $OWNERSHIP_DOC, and a CTE that
#     does shadow a real table is reported as its own violation. Before that, one
#     CTE named after a sibling table silenced every reference to the real table
#     in the whole file, reads and writes alike: a roll-up CTE named `orders` in
#     payment/postgres made every genuine `FROM orders` and `UPDATE orders` in
#     that file invisible, without touching $OWNERSHIP_DOC at all.
#     Per-*statement* CTE scoping would not have fixed it either, and is the
#     wrong shape: SQL's own rules say a non-recursive CTE body does not see the
#     CTE, so `WITH orders AS (SELECT id FROM orders ...)` reads the real table
#     from inside the statement that names the CTE. Refusing the shadowing name
#     outright is both stronger and shorter, and shadowing a table name in a CTE
#     is confusing regardless of boundaries. Three CTEs exist today and none
#     shadow anything: `ancestors` (category, recursive) and `picked`
#     (notification, payment).
#   - `FROM (` opens a subquery or a VALUES list, not a table, so it is
#     skipped: the identifier pattern requires a letter or underscore.
#   - A quoted identifier -- `FROM "products"` -- is read as `products`. The
#     pattern allows one optional double quote after the keyword and strips it.
#   - A captured name of `from`, `join` or `into` is dropped. Those three are
#     *reserved* words in Postgres, so no unquoted table can be called them, and
#     one keyword straight after another is never a table reference -- a column
#     named `into` in `SELECT c.into FROM carts c` otherwise reported the table
#     `from`. This is not the name-based skip-list the paragraph below refuses:
#     a real table cannot hide behind a name the database will not accept.
#     `update`, `truncate` and `copy` are deliberately NOT dropped -- Postgres
#     treats those as non-reserved, so a table really can be called `update`.
#   - Four phrases are deleted before extraction, because in each the keyword is
#     followed by something that is not a table: `FOR UPDATE [SKIP LOCKED]`,
#     `ON CONFLICT ... DO UPDATE SET`, `JOIN LATERAL (`, and `COPY ... FROM
#     STDIN`. Deleting the phrase beats adding `set`, `skip`, `lateral` and
#     `stdin` to a skip-list of names: a name-based skip-list is somewhere a real
#     table can hide, and a phrase-based one is not. The cost is that
#     `FOR UPDATE OF <table>` goes unseen, which is fine -- it is a lock hint on
#     a table the same query has already named.
#   - Only non-test files are scanned, and every directory named `postgres`
#     under a feature is walked, at any depth, not only
#     internal/modules/<feature>/postgres/ itself. A vertical slice's adapter
#     lives at internal/modules/<feature>/<slice>/postgres/, and that SQL is
#     this feature's own adapter as much as the top-level one is -- the table
#     it may name does not change because a slice sits between the feature and
#     its postgres/ directory. Test files legitimately seed and assert against
#     sibling tables to satisfy foreign keys; that is fixture setup, not an
#     architectural crossing.
#   - Skipping tests removed the prose false positives that lived in test names
#     ("removes all items from cart", "returns top products from paid orders"),
#     but not all of them: prose in a *production* string literal still trips
#     this check. `var msg = "update orders failed"` in internal/modules/cart/postgres/
#     reports `orders`. Nothing here can tell that string from a query, so the
#     failure mode is a loud false positive rather than a silent miss -- it is
#     recorded in $OWNERSHIP_DOC rather than papered over, because a check that
#     cries wolf gets disabled.
#   - `pg_constraint`, a Postgres catalog table rather than a domain table,
#     appears only in internal/modules/cart/postgres/repository_test.go, which asserts
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

# known_tables prints every table in $OWNERSHIP_DOC, whoever owns it. Used to
# refuse a CTE that shadows a real table name; check_ownership_doc has already
# proved this set equals the set db/migrations/ creates.
known_tables() {
	printf '%s\n' "$OWNERSHIP_ROWS" | awk '{ print $1 }' | sort -u | tr '\n' ' '
}

# owner_of prints the module owning $1, or nothing if no row claims it.
owner_of() {
	printf '%s\n' "$OWNERSHIP_ROWS" | awk -v t="$1" '$1 == t { print $2; exit }'
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

# sql_text strips Go `//` and SQL `--` line comments, lowercases the result so
# every later pattern can be written case-sensitively (bash 3.2 and BSD tooling
# disagree about case-insensitive flags often enough to avoid relying on them),
# and then collapses all whitespace -- newlines included -- into single spaces so
# that the patterns below see whole statements instead of whole lines.
#
# The order matters. `//` and `--` run to end of *line*, so they must be removed
# while lines still exist; collapsing first would swallow the rest of the file
# into the first comment.
sql_text() {
	sed -e 's://.*::' -e 's/--.*//' "$1" \
		| tr '[:upper:]' '[:lower:]' \
		| tr -s '[:space:]' ' '
}

# sql_table_refs prints the identifier following each FROM / JOIN / INSERT INTO /
# UPDATE / TRUNCATE / COPY. Handles LEFT/INNER/CROSS JOIN and `JOIN x ON ...` for
# free, because it anchors on the JOIN keyword itself rather than trying to parse
# the clause, and catches DELETE FROM and MERGE INTO through `from` and `into`.
#
# Writes count, not just reads. `INSERT INTO another_module_table` is a worse
# violation than a join, and matching only FROM/JOIN would let it through.
# `TRUNCATE` and `COPY` are in the set for the same reason: both write, and both
# were outside it. (`pgx.CopyFrom` still escapes -- it names the table as a Go
# value, not in SQL. Recorded in $OWNERSHIP_DOC.)
#
# Four phrases are deleted first, each a place a keyword is followed by something
# that is not a table: `FOR UPDATE [SKIP LOCKED]`, `ON CONFLICT ... DO UPDATE
# SET`, `JOIN LATERAL (`, and `COPY ... FROM STDIN`. Removing the phrase is safer
# than excusing the words `set`, `skip`, `lateral` and `stdin` by name -- a
# name-based skip-list is a place a real table could hide. The cost is that
# `FOR UPDATE OF <table>` goes unseen, which is fine: it is a lock hint on a
# table the same query has already named.
#
# Those deletions spell their word boundaries as `(^|[^a-z0-9_])` and
# `([^a-z0-9_]|$)` rather than `\b`. BSD sed (macOS) does not implement `\b` and
# does not complain about it either -- it just matches nothing, which would
# quietly restore `set` and `skip` as phantom table names on one platform only.
# `grep -E` below is fine with `\b` on both platforms; sed is not.
#
# The trailing `|| true` matters: under `set -o pipefail` a grep that matches
# nothing (a file with no SQL, or no CTEs) fails the whole pipeline, and an
# assignment such as `x=$(sql_cte_names f)` would then trip `set -e`.
sql_table_refs() {
	sql_text "$1" \
		| sed -E -e 's/(^|[^a-z0-9_])(for|do)[[:space:]]+update([^a-z0-9_]|$)/\1 \3/g' \
			-e 's/(^|[^a-z0-9_])join[[:space:]]+lateral([^a-z0-9_]|$)/\1join \2/g' \
			-e 's/(^|[^a-z0-9_])from[[:space:]]+stdin([^a-z0-9_]|$)/\1 \2/g' \
		| grep -oE '\b(from|join|into|update|truncate|copy)[[:space:]]+"?[a-z_][a-z0-9_]*' \
		| awk '{print $2}' \
		| sed -e 's/^"//' \
		| grep -vxE 'from|join|into' \
		| sort -u || true
}

# sql_cte_names prints CTE names declared in the file: `WITH <name> AS (`,
# `WITH RECURSIVE <name> AS (`, and `, <name> AS (` for chained CTEs. It reads
# the same collapsed text as sql_table_refs on purpose: if a `WITH` and its name
# were allowed to land on separate lines for one function and not the other, the
# CTE would be missed while its uses were still found, and a real CTE would
# report as a cross-module reference.
sql_cte_names() {
	sql_text "$1" \
		| grep -oE '(\bwith[[:space:]]+(recursive[[:space:]]+)?|,[[:space:]]*)[a-z_][a-z0-9_]*[[:space:]]+as[[:space:]]*\(' \
		| sed -E 's/[[:space:]]+as[[:space:]]*\($//' \
		| sed -E 's/^(with[[:space:]]+(recursive[[:space:]]+)?|,[[:space:]]*)//' \
		| sort -u || true
}

# module_go_files prints every non-test .go file under a module, not just the
# ones inside a directory named postgres. Before slicing, a feature's SQL all
# lived in one <feature>/postgres/, so scanning that one directory was scanning
# all of it. Slices put SQL in <feature>/<slice>/postgres/ instead, and there
# is no longer a single privileged directory to point a narrower scan at --
# and SQL was never guaranteed to stay inside postgres/ in the first place, so
# service.go or a slice's command.go naming a table was already invisible to
# the old scan. Widening to the whole module closes that hole for free.
module_go_files() {
	find "$MODULES_ROOT/$1" -name '*.go' ! -name '*_test.go' -type f
}

check_table_ownership() {
	local feature allowed file ref legit found known cte cte_names files

	# With no ownership data there is nothing to check, and looping anyway would
	# bury check_ownership_doc's one accurate diagnosis under a "feature X owns
	# no table" line for every module. Report the cause once; stop there.
	[ -n "$OWNERSHIP_ROWS" ] || return 0

	# Space-delimited on both sides so `case " $known "` cannot match a prefix.
	known=" $(known_tables)"

	while IFS= read -r feature; do
		case " $CHECK_2_EXEMPT_FEATURES " in
		*" $feature "*) continue ;;
		esac

		# A postgres/ directory can sit at the feature root or inside a slice
		# (internal/modules/<feature>/<slice>/postgres/), so presence is a
		# search by name under the whole feature, not a test of one fixed
		# path. Testing only the fixed path was the bug: it skipped a
		# feature's SQL entirely once that feature's adapters moved into
		# slices, and would have skipped the feature outright the day its
		# last top-level postgres/ was deleted. A feature with no postgres/
		# anywhere legitimately owns no table -- auth is today's example --
		# and is still skipped exactly as before.
		[ -n "$(find "$MODULES_ROOT/$feature" -type d -name postgres)" ] || continue

		allowed="$(allowed_tables "$feature")"
		if [ -z "${allowed// /}" ]; then
			report "feature '$feature' has a postgres adapter but owns no table in $OWNERSHIP_DOC
    Add a row per table it owns. If it genuinely owns none, it is either a
    reporting read-model (see the carve-out in $OWNERSHIP_DOC) or it should not
    have a postgres adapter at all."
			continue
		fi

		# Captured into a variable rather than read straight off a
		# `< <(module_go_files ...)` process substitution on purpose: a
		# process substitution's exit status is invisible to its consumer, so
		# a crash inside it reads as "produced zero lines" and the while loop
		# below would finish having checked nothing, same silent-pass shape as
		# the crash in check 3's grep. Assigning to a variable puts the
		# pipeline's status (`set -o pipefail` is on for the whole script)
		# back in a position where `set -e` sees it: a real failure here kills
		# the script loudly instead of reporting Boundaries OK having skipped
		# a feature.
		files="$(module_go_files "$feature" | sort)"
		while IFS= read -r file; do
			[ -f "$file" ] || continue

			# A CTE is not a table, so its name must not be reported as one --
			# but only a name that is not a table can be exempted. A CTE named
			# after a real table is refused outright: exempting it would hide
			# every genuine reference to that table in the file, and shadowing a
			# table name in SQL is confusing on its own merits.
			cte_names=''
			for cte in $(sql_cte_names "$file"); do
				case "$known" in
				*" $cte "*)
					report "$(printf 'CTE shadows a real table: %s declares a CTE named %s, which is the table owned by %s\n    Rename the CTE. A CTE named after a table hides every genuine reference\n    to that table in the file from this check, and hides the real table from\n    the reader of the query. Ownership is recorded in %s.' \
						"$file" "$cte" "$(owner_of "$cte")" "$OWNERSHIP_DOC")"
					;;
				*) cte_names="$cte_names $cte" ;;
				esac
			done
			legit="$allowed $cte_names"

			while IFS= read -r ref; do
				[ -n "$ref" ] || continue

				# Scanning the whole module, not just postgres/, means
				# sql_table_refs now runs over service and command files whose
				# only "SQL" is an slog message: `errorContext(ctx, "failed to
				# update payment status", ...)` matches the update+identifier
				# pattern the same way `UPDATE payments SET ...` does. Requiring
				# the match to also be a real table name -- not merely absent
				# from $legit -- is what tells "payment" (a word) apart from
				# "payments" (the table): the former is never in $known and so
				# never reported, the latter still is. This does not loosen
				# which keywords trigger a match, only which matches are worth
				# reporting; a word that names no table cannot be a table-
				# ownership violation.
				case "$known" in
				*" $ref "*) ;;
				*) continue ;;
				esac

				found=1
				case " $legit " in
				*" $ref "*) found=0 ;;
				esac
				if [ "$found" = 1 ]; then
					report "$(printf 'cross-module table reference: %s names table %s, which %s does not own\n    %s may query: %s\n    Ownership is recorded in %s; cross-module reads go through a port\n    (see ARCHITECTURE.md section 6). Widen the document only to record a\n    decision, never to silence this message.' \
						"$file" "$ref" "$feature" "$feature" "${allowed% }" "$OWNERSHIP_DOC")"
				fi
			done < <(sql_table_refs "$file")
			# Every .go file under the feature, at any depth, not only the
			# ones inside a directory named postgres: a query moved into
			# internal/modules/<feature>/postgres/queries/ was already
			# scanned before this widening, and so was one that lives in
			# internal/modules/<feature>/<slice>/postgres/ -- but SQL sitting
			# in service.go or a slice's command.go, outside any postgres/
			# directory, was not. That was the hole db/OWNERSHIP.md's "what
			# it does not catch" section named; module_go_files closes it.
		done <<<"$files"
	done < <(feature_dirs)
}

# ---------------------------------------------------------------------------
# Check 4 -- a module imports only <feature>/contract from another module
# ---------------------------------------------------------------------------
#
# This replaces the old check 3, which named three adapter package types
# (postgres, http, redis) and asked who imported them. The new rule is one
# sentence and covers strictly more: from another module, only contract/ is
# importable. domain/ is private, every slice -- root package and adapter
# alike -- is private, and contract/ is the whole published surface. Neither
# domain/ nor a slice's bare root package (internal/modules/order/place, with
# no adapter suffix) was nameable by the old check's pattern; this one does
# not need to name what is forbidden, only what is allowed.
#
# Same-module imports are unrestricted here: a slice reaching a sibling slice
# in its own module is not a cross-module import at all -- both packages live
# under the same feature -- and is a separate rule, for a later check to add,
# not this one's job.
#
# Exempt: the wiring layer, and only the wiring layer. internal/bootstrap/ and
# internal/transport/ exist precisely to import adapters and wire them
# together, so they are skipped as importers via WIRING_DIRS (importer_roots
# applies that exemption once, for every check that walks it). Everything else
# under internal/ is scanned, features and shared infrastructure alike -- "not
# a feature" is not the same permission as "may wire adapters", and
# internal/platform must not import product/domain any more than it may
# import product/postgres.
#
# _test.go files are in scope, unlike check_table_ownership's scan. A test
# reaching into a sibling module's slice proves the same coupling a production
# import would, and it is the cheaper place to introduce one.
check_cross_module_imports() {
	local module module_re modules_re feature_alt
	local importers importer importer_name
	local files file file_feature rest
	local hits hit rc imp target target_rest

	module="$(awk '/^module /{print $2; exit}' go.mod)"
	if [ -z "$module" ]; then
		report 'could not read the module path from go.mod'
		return 0
	fi
	# Escape regex metacharacters in the module path ("github.com/..." has dots).
	module_re="$(printf '%s' "$module" | sed -e 's/[.[\*^$\/]/\\&/g')"
	# $MODULES_ROOT is a path with slashes; escape it for the grep below too.
	modules_re="$(printf '%s' "$MODULES_ROOT" | sed -e 's/[.[\*^$\/]/\\&/g')"

	# feature_dirs cannot be empty by the time this runs -- the guard beside its
	# definition exits the script first -- and that guard is what makes this bail
	# safe to keep as belt and braces. On its own it was the silent death of this
	# check: with no features the alternation collapses to `()`, which matches the
	# empty string, so the pattern would demand `internal/modules//contract`, match
	# nothing, and contribute no violations.
	feature_alt="$(feature_dirs | tr '\n' '|' | sed -e 's/|$//')"
	[ -n "$feature_alt" ] || return 0

	# Captured into a variable rather than read straight off a
	# `done < <(importer_roots)` process substitution: importer_roots is cheap
	# and has never been the crashing grep, but capturing it costs nothing and
	# keeps every producer in this check on the same footing as the one below
	# that matters -- a real failure surfaces under `set -e` instead of being
	# read as "importer_roots produced nothing".
	importers="$(importer_roots)"

	while IFS= read -r importer; do
		[ -n "$importer" ] || continue
		importer_name="$(basename "$importer")"

		files="$(find "$importer" -type f -name '*.go' | sort)"
		[ -n "$files" ] || continue

		while IFS= read -r file; do
			[ -f "$file" ] || continue

			# The importing file's own feature, derived from where it sits.
			# Empty for anything outside $MODULES_ROOT -- internal/platform, a
			# stray file directly at the modules root -- which is deliberate:
			# such a file belongs to no module, so it is held to exactly the
			# same contract-only rule as a sibling module would be.
			file_feature=''
			case "$file" in
			"$MODULES_ROOT"/*)
				rest="${file#"$MODULES_ROOT"/}"
				file_feature="${rest%%/*}"
				;;
			esac

			# A status-checked assignment, not `done < <(grep ...)`. This
			# pattern -- a 14-way feature alternation -- is exactly the shape
			# that once made BSD grep exit via SIGTRAP while a process
			# substitution downstream read the death as EOF and reported
			# nothing checked. grep exits 1 for "no match", which is not a
			# failure here; anything greater than 1 is, and is reported rather
			# than silently treated as a clean file.
			rc=0
			hits="$(grep -noE "\"${module_re}/${modules_re}/(${feature_alt})(/[^\"]*)?\"" "$file")" || rc=$?
			if [ "$rc" -gt 1 ]; then
				report "grep exited $rc scanning $file for cross-module imports -- the check could not run on this file, which is not the same as it passing"
				continue
			fi
			[ -n "$hits" ] || continue

			while IFS= read -r hit; do
				[ -n "$hit" ] || continue
				# hit looks like "12:\"<module>/internal/modules/<target>\"" or,
				# with anything past the feature name, ".../<target>/domain",
				# ".../<target>/<slice>", ".../<target>/<slice>/postgres", or
				# ".../<target>/contract" -- the only shape this check accepts
				# from another module.
				imp="${hit#*:}"
				imp="${imp#\"}"
				imp="${imp%\"}"
				rest="${imp#*"$MODULES_ROOT"/}"
				target="${rest%%/*}"

				[ "$target" = "$file_feature" ] && continue

				target_rest="${rest#"$target"}"
				target_rest="${target_rest#/}"
				case "$target_rest" in
				contract | contract/*) continue ;;
				esac

				report "'${importer_name}' imports another module's internals: ${file}:${hit%%:*}
    ${imp}
    A module may import only <feature>/contract from another module -- domain
    types and every slice, root package or adapter alike, are private to the
    module that owns them. Declare a consumer-side port instead (AGENTS.md
    rule 6; e.g. internal/modules/product/inventory.go or
    internal/modules/category/product.go), or add a contract/ package if a
    struct genuinely needs to cross."
			done <<<"$hits"
		done <<<"$files"
	done <<<"$importers"
}

# ---------------------------------------------------------------------------

check_wire_tags
check_ownership_doc
check_table_ownership
check_cross_module_imports

if [ -s "$VIOLATIONS" ]; then
	echo "Architectural boundary violations found:" >&2
	echo >&2
	cat "$VIOLATIONS" >&2
	echo >&2
	echo "See ARCHITECTURE.md and scripts/check-boundaries.sh for the rules." >&2
	exit 1
fi

echo "Boundaries OK"
