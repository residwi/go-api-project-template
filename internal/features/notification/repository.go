package notification

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, n *domain.Notification) error
	Get(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}
