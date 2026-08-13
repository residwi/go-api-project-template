package response

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// One layout for every keyset cursor: a handler drifting onto another would break
// pagination silently.
const cursorTimeFormat = "2006-01-02T15:04:05.999999Z07:00"

func CursorPage[T any](w http.ResponseWriter, rows []T, limit int, keyOf func(T) (time.Time, uuid.UUID)) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	var next string
	if hasMore && len(rows) > 0 {
		ts, id := keyOf(rows[len(rows)-1])
		next = paging.EncodeCursor(ts.Format(cursorTimeFormat), id.String())
	}

	OK(w, paging.NewCursorPageResult(rows, next, hasMore))
}
