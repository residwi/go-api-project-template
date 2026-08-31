package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

type NotificationManager interface {
	List(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}

type Handler struct {
	service NotificationManager
}

func NewHandler(service NotificationManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	notifications, err := h.service.List(r.Context(), uc.UserID, cursor)
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

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	count, err := h.service.CountUnread(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, unreadCountResponse{Count: count})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.MarkRead(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.MarkAllRead(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
