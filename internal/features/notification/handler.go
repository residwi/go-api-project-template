package notification

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type handler struct {
	service *Service
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	cursor := core.ParseCursorPage(r)

	notifications, err := h.service.ListByUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	hasMore := len(notifications) > cursor.Limit
	if hasMore {
		notifications = notifications[:cursor.Limit]
	}

	var nextCursor string
	if hasMore && len(notifications) > 0 {
		last := notifications[len(notifications)-1]
		nextCursor = core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())
	}

	response.Paginated(w, core.NewCursorPageResult(notifications, nextCursor, hasMore))
}

func (h *handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.MarkRead(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	if err := h.service.MarkAllRead(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	count, err := h.service.CountUnread(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, UnreadCountResponse{Count: count})
}
