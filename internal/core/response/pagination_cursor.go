package response

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

// cursorTimeFormat is the single timestamp layout every keyset cursor is built
// with. Keeping it here stops list handlers from drifting onto incompatible
// layouts that would silently break pagination.
const cursorTimeFormat = "2006-01-02T15:04:05.999999Z07:00"

// CursorPage slices rows down to limit, derives the next cursor from the last
// kept row via keyOf, and writes the paginated response. Owning the slice, the
// cursor format, and the writer in one place is what keeps every list endpoint
// consistent.
func CursorPage[T any](w http.ResponseWriter, rows []T, limit int, keyOf func(T) (time.Time, uuid.UUID)) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	var next string
	if hasMore && len(rows) > 0 {
		ts, id := keyOf(rows[len(rows)-1])
		next = core.EncodeCursor(ts.Format(cursorTimeFormat), id.String())
	}

	Paginated(w, core.NewCursorPageResult(rows, next, hasMore))
}
