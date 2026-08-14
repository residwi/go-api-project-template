package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type NotificationReader interface {
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
}

type Handler struct {
	reader NotificationReader
}

func New(reader NotificationReader) *Handler {
	return &Handler{reader: reader}
}

type notificationResponse struct {
	ID        uuid.UUID   `json:"id"`
	Type      domain.Type `json:"type"`
	Title     string      `json:"title"`
	Body      string      `json:"body,omitempty"`
	IsRead    bool        `json:"is_read"`
	CreatedAt time.Time   `json:"created_at"`
}

func toNotificationResponse(n domain.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	notifications, err := h.reader.ListByUser(r.Context(), uc.UserID, cursor)
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

type unreadCountResponse struct {
	Count int `json:"count"`
}

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	count, err := h.reader.CountUnread(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, unreadCountResponse{Count: count})
}
