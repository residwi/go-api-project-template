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

func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func KeysetCursor(where string, args []any, argIdx int, columns, cursor string) (string, []any, int, error) {
	createdAt, id, err := paging.DecodeCursor(cursor)
	if err != nil {
		return where, args, argIdx, fmt.Errorf("%w: invalid cursor", apperror.ErrBadRequest)
	}
	where += fmt.Sprintf(" AND (%s) < ($%d, $%d)", columns, argIdx, argIdx+1)
	args = append(args, createdAt, id)
	return where, args, argIdx + keysetCursorArgs, nil
}
