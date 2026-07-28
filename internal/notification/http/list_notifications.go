package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/notification"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// notificationResponse omits UserID -- the caller is always the
// authenticated user, so echoing it back adds nothing -- and Data, the raw
// payload behind the notification's job. If a client ever needs a piece of
// that payload, it belongs in a new, explicitly typed field on this struct,
// not passed through as raw bytes.
type notificationResponse struct {
	ID        uuid.UUID         `json:"id"`
	Type      notification.Type `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	IsRead    bool              `json:"is_read"`
	CreatedAt time.Time         `json:"created_at"`
}

func toNotificationResponse(n notification.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt,
	}
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	notifications, err := h.service.ListByUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]notificationResponse, len(notifications))
	for i, n := range notifications {
		out[i] = toNotificationResponse(n)
	}

	response.CursorPage(w, out, cursor.Limit, func(n notificationResponse) (time.Time, uuid.UUID) {
		return n.CreatedAt, n.ID
	})
}
