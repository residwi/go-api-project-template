package payment

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// JobRepository is the storage port jobs/postgres satisfies. It is declared
// here -- the consumer's package, per rule 10 -- rather than in jobs/postgres
// itself: Service constructs jobs/postgres directly from Deps.DB (see
// service.go), so the import has to run payment -> jobs/postgres, and a
// compile-time assertion the other way, in jobs/postgres, would cycle back.
type JobRepository interface {
	Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error)
	Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error)
	CreateJob(ctx context.Context, job *domain.Job) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
	MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error
}

// Claim and Prune satisfy platform/jobs.LegacyQueue[domain.Job] directly on
// *Service, so cmd/worker hands app.Payments to jobs.NewLegacyRunner as the queue
// with no separate Queue type standing between them.
func (s *Service) Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error) {
	return s.jobs.Claim(ctx, batchSize, leaseDuration)
}

func (s *Service) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	return s.jobs.Prune(ctx, olderThan, limit)
}

// CancelPendingByOrderID satisfies checkout's Payments port.
func (s *Service) CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return s.jobs.CancelJobsByOrderID(ctx, orderID)
}

// enqueueRefund is shared by every internal caller that needs a refund job
// created: Refund's own request, FinalizeSuccess's late-payment branch, and
// CompensateRefund.
func (s *Service) enqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	return s.jobs.CreateJob(ctx, &domain.Job{
		PaymentID:   paymentID,
		OrderID:     orderID,
		Action:      domain.ActionRefund,
		Status:      domain.JobStatusPending,
		NextRetryAt: time.Now(),
	})
}
