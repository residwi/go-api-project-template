package notification

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]Notification, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	CreateJob(ctx context.Context, job *Job) error
	Claim(ctx context.Context, batchSize int, lease time.Duration) ([]Job, error)
	UpdateJob(ctx context.Context, job *Job) error
	CreateAndComplete(ctx context.Context, n *Notification, job *Job) error
	Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error)
}
