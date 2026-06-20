package database

import (
	"fmt"
	"strings"

	"github.com/residwi/go-api-project-template/internal/core"
)

// keysetCursorArgs is the number of placeholders a keyset predicate appends:
// the createdAt and id bounds of the cursor.
const keysetCursorArgs = 2

// EscapeLike escapes LIKE/ILIKE metacharacters so user-supplied search text is
// matched literally rather than as wildcards. Postgres treats backslash as the
// default escape character, so it is escaped first; NewReplacer applies all
// rules in a single pass, which avoids double-escaping.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// KeysetCursor decodes a keyset pagination cursor and appends a predicate that
// pages backwards over (createdAt, id) to the given WHERE clause and args.
//
// columns is the keyset tuple expression — "created_at, id" for a single table,
// or a qualified form like "wi.created_at, wi.id" when the query joins. It is
// interpolated into the SQL verbatim (not a bind parameter), so it MUST be a
// trusted compile-time literal — never pass user- or request-derived text. The
// returned argIdx is advanced past the two placeholders that were appended. A
// malformed cursor yields core.ErrBadRequest and leaves the inputs unchanged.
func KeysetCursor(where string, args []any, argIdx int, columns, cursor string) (string, []any, int, error) {
	createdAt, id, err := core.DecodeCursor(cursor)
	if err != nil {
		return where, args, argIdx, fmt.Errorf("%w: invalid cursor", core.ErrBadRequest)
	}
	where += fmt.Sprintf(" AND (%s) < ($%d, $%d)", columns, argIdx, argIdx+1)
	args = append(args, createdAt, id)
	return where, args, argIdx + keysetCursorArgs, nil
}
