#!/usr/bin/env bash
#
# check-boundaries.sh -- turn Phase 4's module boundaries into a build failure
# instead of a paragraph in a plan document.
#
# Six checks run, numbered 1, 2, 3, 4, 6 and 8. The gaps are where checks 5
# and 7 were retired and are deliberate: renumbering would falsify every
# by-number citation in AGENTS.md, ARCHITECTURE.md and db/OWNERSHIP.md at
# once, and a gap states something true -- a check used to be here -- that a
# closed list would hide.
#
#   Check 1  Wire (`json:`) tags live only in a module's http adapter.
#   Check 2  db/OWNERSHIP.md itself has no duplicate rows, no row for a
#            table no migration creates, and no table with no owning row.
#   Check 3  A feature's SQL -- anywhere under the module, not only its
#            postgres adapter -- only queries tables it owns, where "owns"
#            is read out of db/OWNERSHIP.md at run time.
#   Check 4  A module may not import another module's domain/ or its
#            adapter/ (postgres, http, redis, jobs). Its root package is
#            importable; that is the published surface.
#   Check 5  RETIRED. It refused a slice importing a sibling slice. No module
#            has a usecase/ tree left for it to walk, so it could only ever
#            pass.
#   Check 6  A module may not import internal/server/, except its own
#            adapter/http.
#   Check 7  RETIRED. It kept each contract/ package a leaf -- stdlib, uuid
#            and money only. No module has a contract/ package: the types
#            those held are declared in contract.go in the module's own root
#            package, which imports domain/ by design, so there is nothing
#            left for the rule to be true of.
#   Check 8  internal/platform may import no local package but
#            internal/platform itself, because it is the bottom ring and
#            must compile on its own in a fresh module. internal/testutil is
#            the one exemption, and check 8's own comment says what that
#            costs.
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

# The bottom ring, walked by check 8. Held in a variable for the same reason
# MODULES_ROOT is: check 8 names this path three times -- the directory it
# walks, the prefix that makes an import intra-platform and therefore legal,
# and the advice it prints -- and three literals drift where one does not.
PLATFORM_ROOT='internal/platform'

# The directories whose entire job is to import adapters and wire them together.
# Only these are exempt from check 4 as importers. This one stays a list because
# it is a genuine permission grant, not a classification: it should be short, and
# adding to it should be a visible, argued diff.
#
# checkout is deliberately NOT on this list, even though it needs to import
# order/domain: a blanket importer exemption would also let it import any
# module's postgres adapter with nothing left to catch it. Its one real need
# -- order/domain -- is granted narrowly inside check_cross_module_imports
# below instead, so everything else checkout might reach for is still scanned
# like any other feature.
#
# Held as a full path rather than the bare name (bootstrap, server) this
# held before: importer_roots' two loops walk directories at different
# depths, and is_wiring now takes whichever full path each loop already
# prints, so a future entry naming a module (internal/modules/<feature>)
# works the same way an entry naming a top-level directory does, without a
# depth-specific comparison. Nothing in $MODULES_ROOT sits on this list
# today -- see the checkout paragraph above.
WIRING_DIRS='internal/bootstrap internal/server'

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
		is_wiring "internal/$name" && continue
		printf '%s\n' "internal/$name"
	done
	for dir in "$MODULES_ROOT"/*/; do
		[ -d "$dir" ] || continue
		is_wiring "${dir%/}" && continue
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
# Phase 4 moved every feature's wire DTOs into the http adapter beside the
# handler that serialises them, today internal/modules/<feature>/adapter/http/.
# A `json:` tag on a domain model means the model has started doubling as a
# transport type again, which is what the phase existed to undo.
#
# Exempt by location:
#   internal/modules/<feature>/adapter/http/*.go
#                       the wire adapters -- this is where tags belong. A path
#                       short of this pattern
#                       (internal/modules/<feature>/http/) is deliberately NOT
#                       exempt: that path held the feature route tables until
#                       they moved to internal/server/routes.go, and a
#                       json tag reappearing there would mean a DTO had drifted
#                       out of the module that owns it. Likewise
#                       internal/server/ (the top-level router) is not
#                       exempt: it wires modules together and defines no wire
#                       types of its own. `is_http_adapter` below is the one
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
#   internal/modules/payment/adapter/gateway/gateway.go
#     ChargeRequest / ChargeResponse / RefundRequest / RefundResponse are the
#     *external* payment gateway's wire contract, not this system's. The tags
#     describe someone else's API. Only this one file needs the exemption --
#     gateway/stripe, gateway/midtrans and gateway/mock call out to it with
#     these same structs but declare none of their own, so allowlisting the
#     whole gateway/ directory would exempt three files that carry zero json
#     tags today, on the strength of one that does. This was a single file
#     at the module root (payment/gateway.go) before payment was sliced into
#     vertical slices; moving it under payment/gateway/, then under
#     payment/adapter/gateway/, still narrows the exemption, because a json
#     tag on a domain type in payment's own model can no longer land in the
#     same file as this one by accident -- but the allowlist entry itself
#     stays exactly as wide as the one file that earns it. An unexplained
#     exemption in a lint rule is how the rule erodes, so this one carries
#     its reason.
#
#   internal/server/response/response.go used to need an entry here too:
#     Response and Error are the shared envelope every handler in every
#     module writes through. Task 3 of the platform/transport split moved the
#     file to internal/platform/response/response.go, which is exempt by
#     location already -- internal/platform/ above -- so the allowlist entry
#     is gone with it rather than pointing at a path nothing owns any more.
JSON_TAG_ALLOWLIST='
internal/modules/payment/adapter/gateway/gateway.go
'

is_json_tag_allowlisted() {
	printf '%s\n' "$JSON_TAG_ALLOWLIST" | grep -qxF -- "$1"
}

# is_http_adapter is true for a module's own http adapter --
# internal/modules/<feature>/adapter/http/*.go -- and nothing else. A path
# short of that pattern (internal/modules/<feature>/http/*.go) correctly
# returns false: no such directory exists, and nothing may recreate one to
# hold a DTO. The second arm this carried, matching a slice's
# usecase/<slice>/http/, went with the slices; while it lasted it was the
# only exemption in this script that could not be reached from any path in
# the tree. This one arm carries every module: drop it and check 1 reports
# 294 tags in fifteen adapters at once.
is_http_adapter() {
	case "$1" in
	"$MODULES_ROOT"/*/adapter/http/*.go) return 0 ;;
	esac
	return 1
}

check_wire_tags() {
	local file line

	# 1a. json: tags on types this system owns, outside a module's http adapter.
	while IFS= read -r file; do
		is_http_adapter "$file" && continue
		is_json_tag_allowlisted "$file" && continue
		while IFS= read -r line; do
			report "json tag outside an http adapter: ${file}:${line%%:*}
    Wire DTOs belong in internal/modules/<feature>/adapter/http/. Domain models carry no json tags.
    If this type really is someone else's wire contract, add it to
    JSON_TAG_ALLOWLIST in scripts/check-boundaries.sh with a reason."
		done < <(grep -n 'json:"' "$file" || true)
	done < <(find internal -type f -name '*.go' \
		! -name '*_test.go' \
		! -path 'internal/platform/*' \
		| sort)

	# 1b. json:"-" anywhere under internal/ outside an http adapter.
	#
	# Filtered on /adapter/http/ rather than the looser /http/ this used to
	# carry: with the slices gone there is exactly one kind of http adapter,
	# and a filter wider than the rule is a place a tag could hide.
	#
	# Phase 4 replaced all 14 of these with omission from a DTO. The point is
	# that a field is now private *by default* rather than private by someone
	# remembering to write a tag. This must stay at zero -- no allowlist, and
	# no exemption for tests, config or platform.
	# `grep -rn` prints file:line:content; keep the first two fields.
	while IFS= read -r loc; do
		report "json:\"-\" found outside an http adapter: ${loc}
    Phase 4 replaced every json:\"-\" with omission from a DTO. A field is
    private because no DTO exposes it, not because a tag hides it."
	done < <(grep -rn 'json:"-"' --include='*.go' internal/ \
		| grep -v '/adapter/http/' \
		| cut -d: -f1,2 || true)

	# 1c. A feature's dto.go must not come back.
	#
	# These files were deleted in Phase 4; their contents now live beside the
	# handler that serialises them, in internal/modules/<feature>/adapter/http/.
	#
	# No -mindepth/-maxdepth. They used to pin this to internal/<feature>/dto.go
	# at exactly depth 2, and Phase 6 moved the features to depth 3 -- a depth
	# bound would have kept exiting 0 while checking nothing, which is the one
	# failure mode of a lint rule that nobody notices. Any dto.go anywhere under
	# internal/ is the thing being refused, so the check says exactly that.
	while IFS= read -r file; do
		report "resurrected DTO file: ${file}
    dto.go was deleted in Phase 4. Wire types live in
    internal/modules/<feature>/adapter/http/ next to the handler that serialises them."
	done < <(find internal -type f -name 'dto.go' | sort)
}

# ---------------------------------------------------------------------------
# Checks 2 and 3 -- the ownership document is sound, and a feature's SQL only
# names tables it owns
# ---------------------------------------------------------------------------
#
# Two checks share this section because they share the parser below: check 2
# (check_ownership_doc) validates db/OWNERSHIP.md against db/migrations/, and
# check 3 (check_table_ownership) holds each module's SQL to the rows that
# document gives it. Check 3 is worthless if check 2 has not already proved
# the document is neither empty nor stale, which is why they run in that
# order.
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
#     wraps SQL mid-statement (internal/modules/inventory/adapter/postgres/repository.go ends a
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
#     payment/adapter/postgres made every genuine `FROM orders` and `UPDATE orders` in
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
#   - Only non-test files are scanned, and every .go file under a feature is
#     walked, at any depth, not only the ones inside a directory named
#     `postgres`. A module's SQL sits in
#     internal/modules/<feature>/adapter/postgres/ today, but nothing makes it
#     stay there: a query in service.go names a table just as effectively.
#     Test files legitimately seed and assert against
#     sibling tables to satisfy foreign keys; that is fixture setup, not an
#     architectural crossing.
#   - Skipping tests removed the prose false positives that lived in test names
#     ("removes all items from cart", "returns top products from paid orders"),
#     but not all of them: prose in a *production* string literal still trips
#     this check. `var msg = "update orders failed"` in internal/modules/cart/adapter/postgres/
#     reports `orders`. Nothing here can tell that string from a query, so the
#     failure mode is a loud false positive rather than a silent miss -- it is
#     recorded in $OWNERSHIP_DOC rather than papered over, because a check that
#     cries wolf gets disabled.
#   - `pg_constraint`, a Postgres catalog table rather than a domain table,
#     appears only in internal/modules/cart/adapter/postgres/repository_test.go, which asserts
#     each foreign key's ON DELETE action. Because tests are out of scope it
#     needs no allowlist entry -- recorded here so the next reader does not
#     rediscover it as a violation.
#
# `dashboard` is exempt from check 3 entirely. That is a deliberate
# architectural decision, not an oversight. ARCHITECTURE.md section 6 states
# the carve-out directly: "dashboard is a reporting read-model and may
# read-only join across anything. Expressing a revenue aggregate as
# cross-module service calls instead of a GROUP BY would be slower *and* less
# correct."
CHECK_3_EXEMPT_FEATURES='dashboard'

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

# migration_tables prints every table db/migrations/ still leaves standing.
# Only each file's `-- +goose Up` section is read: a Down section's DROP TABLE
# list mirrors the Up's CREATE TABLE list, and counting both would make every
# table look like it had been created twice. A table an Up section drops --
# not that same file's own Down, a later file's Up, retiring an earlier one's
# CREATE TABLE for good -- is removed from the set rather than counted.
migration_tables() {
	awk '
		/^-- \+goose Up/   { section = "up";   next }
		/^-- \+goose Down/ { section = "down"; next }
		section == "up" && /CREATE TABLE/ {
			if (match($0, /CREATE TABLE[[:space:]]+(IF NOT EXISTS[[:space:]]+)?[a-z_][a-z0-9_]*/)) {
				name = substr($0, RSTART, RLENGTH)
				sub(/^CREATE TABLE[[:space:]]+(IF NOT EXISTS[[:space:]]+)?/, "", name)
				created[name] = 1
			}
		}
		section == "up" && /DROP TABLE/ {
			if (match($0, /DROP TABLE[[:space:]]+(IF EXISTS[[:space:]]+)?[a-z_][a-z0-9_]*/)) {
				name = substr($0, RSTART, RLENGTH)
				sub(/^DROP TABLE[[:space:]]+(IF EXISTS[[:space:]]+)?/, "", name)
				delete created[name]
			}
		}
		END { for (name in created) print name }
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
# ones inside a directory named postgres. A module's SQL does live in one place
# today -- <feature>/adapter/postgres/ -- but nothing enforces that, and a
# table named from service.go was invisible to the narrower scan this replaced.
# Walking the whole module closes that hole for free.
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
		case " $CHECK_3_EXEMPT_FEATURES " in
		*" $feature "*) continue ;;
		esac

		# Presence of a postgres adapter is a search by name under the whole
		# feature, not a test of one fixed path. Testing a fixed path is what
		# broke this before: <feature>/postgres/ was hard-coded, so the check
		# skipped a feature's SQL the day that directory moved -- first under
		# a slice, now under adapter/. A feature with no postgres/ anywhere
		# legitimately owns no table (auth, checkout and money today) and is
		# skipped.
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
			# ones inside a directory named postgres: SQL sitting in
			# service.go, outside any postgres/ directory, was the hole
			# db/OWNERSHIP.md's "what it does not catch" section named, and
			# module_go_files closes it.
		done <<<"$files"
	done < <(feature_dirs)
}

# ---------------------------------------------------------------------------
# Check 4 -- a module may not import another module's domain/ or adapter/
# ---------------------------------------------------------------------------
#
# Collapsing every module's contract/ package into a contract.go at its own
# root means a consumer that used to import <feature>/contract now imports
# <feature> itself: a module's root package is the published surface, and it
# is importable. What stays private is domain/ (the rich model) and every
# adapter (postgres, http, redis, jobs) -- the things a producer's root
# package was never meant to expose just because its own package became
# reachable.
#
# The cost of that trade is not enforced here and cannot be: a module's root
# package publishes every exported method on its Service, so nothing stops
# payment naming order.Place. This check draws the line at the package, not
# at the method.
#
# Same-module imports are unrestricted here, and nothing checks them any more:
# a module's own adapter importing its root package is the target shape, and
# check 5, which used to police one same-module case, is retired.
#
# Exempt as an importer: the wiring layer, and only the wiring layer.
# internal/bootstrap/ and internal/server/ exist precisely to import
# adapters and wire them together, so they are skipped as importers via
# WIRING_DIRS (importer_roots applies that exemption once, for every check
# that walks it). Everything else under internal/ is scanned, features and
# shared infrastructure alike -- "not a feature" is not the same permission
# as "may wire adapters", and internal/platform must not import
# product/domain any more than it may import product/adapter/postgres.
#
# checkout is scanned as an importer like every other feature -- it is not on
# WIRING_DIRS -- but is granted one narrow target exemption below: importing
# order/domain, and only domain/, never an adapter. That exemption is
# load-bearing, not historical: order.Service.Place takes an
# order/domain.NewOrder and returns an *order/domain.Order, so checkout's own
# Orders port has to name both types, and seven files under checkout
# import order/domain today. Retiring it means moving those two types into
# order's root package first, which is a change to two modules rather than to
# this script.
#
# _test.go files are in scope, unlike check_table_ownership's scan. A test
# reaching into a sibling module's internals proves the same coupling a
# production import would, and it is the cheaper place to introduce one.
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
	# check: with no features the alternation collapses to `()`, which matches
	# the empty string, so the pattern would demand a doubled slash --
	# `internal/modules//` -- match nothing, and contribute no violations.
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
			# same rule a sibling module is: root packages yes, domain/ and
			# adapters no.
			file_feature=''
			case "$file" in
			"$MODULES_ROOT"/*)
				rest="${file#"$MODULES_ROOT"/}"
				file_feature="${rest%%/*}"
				;;
			esac

			# A status-checked assignment, not `done < <(grep ...)`. This
			# pattern -- a sixteen-way feature alternation -- is exactly the shape
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
				# hit looks like "12:\"<module>/internal/modules/<target>\"" --
				# a bare import of the target's root package, which is the
				# published surface and allowed -- or, with anything past the
				# feature name, ".../<target>/domain",
				# ".../<target>/adapter/postgres", ".../<target>/jobs" and so
				# on: all private, all reported.
				imp="${hit#*:}"
				imp="${imp#\"}"
				imp="${imp%\"}"
				rest="${imp#*"$MODULES_ROOT"/}"
				target="${rest%%/*}"

				[ "$target" = "$file_feature" ] && continue

				target_rest="${rest#"$target"}"
				target_rest="${target_rest#/}"
				# Empty target_rest is a bare import of the target's root
				# package: importable, because that is the module's published
				# surface. The `contract | contract/*` arm this carried went
				# with the last contract/ directory -- no path under
				# internal/modules can match it any more, so keeping it could
				# only excuse a directory nobody is allowed to recreate.
				case "$target_rest" in
				"") continue ;;
				esac

				# checkout alone may reach into another module's domain/ --
				# and only domain/, never an adapter. order.Service.Place's
				# own signature names order/domain.NewOrder and
				# order/domain.Order, so checkout's port cannot avoid them.
				# See the header comment above this function for why this is a
				# per-importer exemption rather than a WIRING_DIRS entry. The
				# test keys on the importer, so the grant is open on the
				# target: checkout may reach any module's domain/, though
				# order's is the only one it needs today.
				if [ "$file_feature" = "checkout" ]; then
					case "$target_rest" in
					domain | domain/*) continue ;;
					esac
				fi

				report "'${importer_name}' imports another module's internals: ${file}:${hit%%:*}
    ${imp}
    A module may import another module's root package -- that is its
    published surface. domain/ and every adapter (postgres, http, redis,
    jobs) stay private to the module that owns them. Declare a consumer-side
    port instead (AGENTS.md rule 10; e.g. internal/modules/category/ports.go
    or internal/modules/checkout/ports.go), or declare the struct that has to
    cross in the producing module's own contract.go."
			done <<<"$hits"
		done <<<"$files"
	done <<<"$importers"
}

# ---------------------------------------------------------------------------
# Check 6 -- the transport arrow points one way
# ---------------------------------------------------------------------------
#
# A module may not import internal/server/. Only its own adapter/http may,
# because that package exists to speak HTTP and nothing constructs it except
# the route table.
#
# That one exemption carries every module: adapter/http is where
# response.Bind and middleware.RequireUser are called (AGENTS.md rule 18), so
# dropping it reports 85 imports across fifteen modules in one run. The second
# arm this case used to hold, matching a slice's usecase/<slice>/http/, went
# with the slices and matched nothing while it lasted.
#
# Two things this catches that a service-file-only check would miss:
#   - a service returning a transport type, which is how user.Service came to
#     return middleware.UserStatusResult and auth.Service *middleware.TokenClaims
#     (both fixed in phase 0, before this check existed to stop the third)
#   - a module registering routes, which would make every binary constructing
#     it link HTTP -- including the worker, which serves nothing
#
# Go imports are per-package, so splitting service.go into service.go plus
# http.go would not help: one import of the module pulls every file in it.
#
# A module that needs to describe something the transport also describes
# declares the type in its own contract.go and lets middleware import the
# module root instead. That is what user.AccountStatus and auth.ClaimsView
# are for.
#
# Matching is on a quoted import path, not on any occurrence of the text --
# a doc comment explaining why a setter was removed could say it "would
# compile from cmd/, from internal/server/, from any test", and that
# sentence contains the string "internal/server" with no surrounding
# quotes and no module prefix. A check that grepped for the bare text would
# flag a comment that imports nothing. Requiring the full module path inside
# quotes -- the same shape check_cross_module_imports already requires for its
# own cross-module greps -- only matches a real import.
check_transport_direction() {
	local module module_re
	local feature files file rc hits hit

	module="$(awk '/^module /{print $2; exit}' go.mod)"
	if [ -z "$module" ]; then
		report 'could not read the module path from go.mod'
		return 0
	fi
	# Escaped the same way check 4 escapes it: github.com/... has dots,
	# which are regex metacharacters.
	module_re="$(printf '%s' "$module" | sed -e 's/[.[\*^$\/]/\\&/g')"

	while IFS= read -r feature; do
		[ -n "$feature" ] || continue

		files="$(find "$MODULES_ROOT/$feature" -type f -name '*.go' | sort)"
		[ -n "$files" ] || continue

		while IFS= read -r file; do
			[ -f "$file" ] || continue

			# A module's own http adapter
			# (internal/modules/<feature>/adapter/http/) is the one legitimate
			# importer -- it speaks HTTP by design. The feature route tables
			# that used to be a second exemption are gone: every URL now lives
			# in internal/server/routes.go, so no file under a module names a
			# route or needs middleware.RouteGroup.
			case "$file" in
			"$MODULES_ROOT/$feature"/adapter/http/*.go) continue ;;
			esac

			# A status-checked assignment, not `done < <(grep ...)` -- the shape
			# that once made BSD grep exit via SIGTRAP while a downstream process
			# substitution read the death as EOF and reported nothing checked
			# (see the note above check_cross_module_imports). grep exits 1 for
			# "no match", which is not a failure here; anything greater than 1
			# is, and is reported rather than silently treated as a clean file.
			rc=0
			hits="$(grep -noE "\"${module_re}/internal/server(/[^\"]*)?\"" "$file")" || rc=$?
			if [ "$rc" -gt 1 ]; then
				report "grep exited $rc scanning $file for internal/server imports -- the check could not run on this file, which is not the same as it passing"
				continue
			fi
			[ -n "$hits" ] || continue

			while IFS= read -r hit; do
				[ -n "$hit" ] || continue
				report "'${feature}' imports internal/server: ${file}:${hit%%:*}
    A module may not import internal/server -- only its own adapter/http
    may, because that package exists to speak HTTP and nothing constructs it
    but the route table. Declare the shared type in
    ${MODULES_ROOT}/${feature}/contract.go and let the transport import the
    module root instead."
			done <<<"$hits"
		done <<<"$files"
	done < <(feature_dirs)
}

# ---------------------------------------------------------------------------
# Check 8. internal/platform is the bottom ring: it must copy into a fresh
# module and compile. That holds only while nothing under it reaches upward,
# so any import of this repository's own code from a platform file is a
# violation unless the target is platform itself. Numbering continues at 8
# because 5 and 7 were retired and renumbering would falsify every by-number
# citation.
#
# The test is deliberately inverted -- match every local import, then subtract
# what is allowed -- rather than naming the trees that must not be imported.
# The first version of this check named three (internal/modules,
# internal/server, internal/apperror) and so said nothing about
# internal/bootstrap or cmd/mockgateway/mockserver, either of which would end
# the leaf property just as completely while passing silently. A closed list
# also has to be remembered: it covers a tree added later only on the day
# someone thinks to extend it. Subtracting from "everything local" covers that
# tree the day it exists.
#
# internal/testutil is the single exemption, and it is a real hole rather than
# a tidy one. Three platform test packages import it for the shared dockertest
# harness -- platform/database, platform/cache and platform/jobs/postgres --
# and internal/testutil does not live under internal/platform, so it does not
# travel with a copy of it. That is why the copy property this check defends
# holds for `go build` on a copied internal/platform but NOT for `go test`:
# the non-test build is a leaf, the test build is not. Pre-existing, not
# introduced by the split that added this check, and closing it means moving
# internal/testutil under internal/platform. Until someone does, it is a stated
# limitation rather than an oversight -- which is the whole reason it is
# written down here instead of being an unexplained gap in a pattern.
check_platform_leaf() {
	local module files file rc hits hit path

	module="$(awk '/^module /{print $2; exit}' go.mod)"
	if [ -z "$module" ]; then
		report 'could not read the module path from go.mod'
		return 0
	fi

	files="$(find "$PLATFORM_ROOT" -type f -name '*.go' | sort)"
	if [ -z "$files" ]; then
		report "check 8 found no files under $PLATFORM_ROOT -- the check could not run, which is not the same as it passing"
		return 0
	fi

	while IFS= read -r file; do
		[ -f "$file" ] || continue

		# Matched as a fixed string, not a regex: the module path is the whole
		# left-hand side here, so there is nothing to escape and nothing an
		# unescaped metacharacter could quietly widen. Every import of local
		# code is a hit; the case block below decides which ones are legal.
		rc=0
		hits="$(grep -nF "\"${module}/" "$file")" || rc=$?
		if [ "$rc" -gt 1 ]; then
			report "grep exited $rc scanning $file for upward imports -- the check could not run on this file, which is not the same as it passing"
			continue
		fi
		[ -n "$hits" ] || continue

		while IFS= read -r hit; do
			[ -n "$hit" ] || continue

			# Whole matching lines, not -o matches: the import path has to be
			# read out to decide whether it is legal, and the quoted string on
			# the line is exactly that path, alias or no alias.
			path="${hit#*:}"
			path="${path#*\"}"
			path="${path%%\"*}"
			path="${path#"$module"/}"

			case "$path" in
			"$PLATFORM_ROOT" | "$PLATFORM_ROOT"/*) continue ;;
			internal/testutil | internal/testutil/*) continue ;;
			esac

			report "platform must not import upward: ${file}:${hit%%:*} imports ${path}
    ${PLATFORM_ROOT} is the bottom ring and must compile on its own in a
    fresh module, so it may import stdlib, third-party packages and
    ${PLATFORM_ROOT} itself, and nothing else of this repository's.
    Declare what you need inside ${PLATFORM_ROOT}, or move the consumer up
    into internal/server."
		done <<<"$hits"
	done <<<"$files"
}

# ---------------------------------------------------------------------------

check_wire_tags
check_ownership_doc
check_table_ownership
check_cross_module_imports
check_transport_direction
check_platform_leaf

if [ -s "$VIOLATIONS" ]; then
	echo "Architectural boundary violations found:" >&2
	echo >&2
	cat "$VIOLATIONS" >&2
	echo >&2
	echo "See ARCHITECTURE.md and scripts/check-boundaries.sh for the rules." >&2
	exit 1
fi

echo "Boundaries OK"
