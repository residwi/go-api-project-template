package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	cursor := paging.ParseCursorPage(r)

	notifications, err := h.service.List(r.Context(), uc.UserID, cursor)
	if err != nil {
		return err
	}

	out := make([]notificationResponse, len(notifications))
	for i, n := range notifications {
		out[i] = toNotificationResponse(n)
	}

	response.CursorPage(w, out, cursor.Limit, func(n notificationResponse) (time.Time, uuid.UUID) {
		return n.CreatedAt, n.ID
	})
	return nil
}

func (h *Handler) CountUnread(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	count, err := h.service.CountUnread(r.Context(), uc.UserID)
	if err != nil {
		return err
	}

	response.OK(w, unreadCountResponse{Count: count})
	return nil
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.MarkRead(r.Context(), uc.UserID, id); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	if err := h.service.MarkAllRead(r.Context(), uc.UserID); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
