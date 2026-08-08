package jobs

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
)

// Repository is jobs' own storage: the queue table plus the notifications
// table Process writes into when a job completes. Its only implementation is
// jobs/postgres, constructed in notification/module.go.
type Repository interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	Claim(ctx context.Context, batch int, lease time.Duration) ([]domain.Job, error)
	Prune(ctx context.Context, age time.Duration, limit int) (int, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	CreateAndComplete(ctx context.Context, n *domain.Notification, job *domain.Job) error
}
