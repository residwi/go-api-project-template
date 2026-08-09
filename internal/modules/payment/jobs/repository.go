package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// Repository is jobs' own storage: every operation on the payment_jobs queue
// table. Charge, refund and webhook never touch payment_jobs directly -- they
// reach it only through the exported methods on Command -- the same rule
// notification/jobs follows for its own queue table.
type Repository interface {
	Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error)
	Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error)
	CreateJob(ctx context.Context, job *domain.Job) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
	MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error
}
