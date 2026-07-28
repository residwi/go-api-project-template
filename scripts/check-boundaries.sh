#!/usr/bin/env bash
#
# check-boundaries.sh -- turn Phase 4's module boundaries into a build failure
# instead of a paragraph in a plan document.
#
#   Check 1  Wire (`json:`) tags live only in a feature's http adapter.
#   Check 2  A feature's postgres adapter only queries tables it owns.
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
# The per-feature sets below are deliberately derived from what the current
# tree actually references, and will be tightened against db/OWNERSHIP.md in
# Phase 5, where the ownership table gets written down properly. The purpose
# of this check is to catch an *accidental new* cross-module join in review,
# not to relitigate the documented Phase 2 exceptions.
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

# allowed_tables prints the tables a feature's postgres adapter may name.
# An empty result means the feature is not known to this check, which is
# itself a failure: a new postgres adapter must declare what it owns.
allowed_tables() {
	case "$1" in
	cart) echo 'carts cart_items' ;;
	category) echo 'categories' ;;
	inventory) echo 'inventory_levels' ;;
	notification) echo 'notifications notification_jobs' ;;
	order) echo 'orders order_items' ;;
	payment) echo 'payments payment_jobs' ;;
	product) echo 'products product_images' ;;
	promotion) echo 'promotions coupon_usages' ;;
	review) echo 'reviews' ;;
	shipping) echo 'shipments' ;;
	user) echo 'users' ;;
	wishlist) echo 'wishlists wishlist_items' ;;
	*) echo '' ;;
	esac
}

# strip_comments removes Go `//` and SQL `--` line comments, and lowercases
# the result so every later pattern can be written case-sensitively (bash 3.2
# and BSD tooling disagree about case-insensitive flags often enough to avoid
# relying on them).
strip_comments() {
	sed -e 's://.*::' -e 's/--.*//' "$1" | tr '[:upper:]' '[:lower:]'
}

# sql_table_refs prints the identifier following each FROM / JOIN. Handles
# LEFT/INNER/CROSS JOIN and `JOIN x ON ...` for free, because it anchors on
# the JOIN keyword itself rather than trying to parse the clause.
# The trailing `|| true` matters: under `set -o pipefail` a grep that matches
# nothing (a file with no SQL, or no CTEs) fails the whole pipeline, and an
# assignment such as `x=$(sql_cte_names f)` would then trip `set -e`.
sql_table_refs() {
	strip_comments "$1" \
		| grep -oE '\b(from|join)[[:space:]]+[a-z_][a-z0-9_]*' \
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

	while IFS= read -r feature; do
		case " $CHECK_2_EXEMPT_FEATURES " in
		*" $feature "*) continue ;;
		esac
		[ -d "internal/$feature/postgres" ] || continue

		allowed="$(allowed_tables "$feature")"
		if [ -z "$allowed" ]; then
			report "feature '$feature' has a postgres adapter but no entry in allowed_tables()
    Declare the tables it owns in scripts/check-boundaries.sh."
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
					report "$(printf 'cross-module table reference: %s names table %s, which %s does not own\n    %s may query: %s\n    Cross-module reads go through a port (see ARCHITECTURE.md section 6).' \
						"$file" "$ref" "$feature" "$feature" "$allowed")"
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
