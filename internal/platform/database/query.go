package database

import (
	"fmt"
	"strings"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// keysetCursorArgs is the number of placeholders a keyset predicate appends:
// the createdAt and id bounds of the cursor.
const keysetCursorArgs = 2

// EscapeLike makes user-supplied search text match literally, not as wildcards.
// Backslash goes first (Postgres's default escape character) and NewReplacer runs
// every rule in one pass, which is what avoids double-escaping.
func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// KeysetCursor appends a predicate paging backwards over (createdAt, id).
//
// columns is interpolated into the SQL verbatim, not bound, so it MUST be a
// trusted compile-time literal -- never user- or request-derived text. A
// malformed cursor yields apperror.ErrBadRequest and changes nothing.
func KeysetCursor(where string, args []any, argIdx int, columns, cursor string) (string, []any, int, error) {
	createdAt, id, err := paging.DecodeCursor(cursor)
	if err != nil {
		return where, args, argIdx, fmt.Errorf("%w: invalid cursor", apperror.ErrBadRequest)
	}
	where += fmt.Sprintf(" AND (%s) < ($%d, $%d)", columns, argIdx, argIdx+1)
	args = append(args, createdAt, id)
	return where, args, argIdx + keysetCursorArgs, nil
}
