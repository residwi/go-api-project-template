package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
)

type notificationResponse struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

func toNotificationResponse(n domain.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID,
		Title:     n.Title,
		Body:      n.Body,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt,
	}
}

type unreadCountResponse struct {
	Count int `json:"count"`
}
