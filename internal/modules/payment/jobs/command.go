// Package jobs owns the payment_jobs queue table and every operation on it.
// Command satisfies platform/jobs.Queue (Claim, Prune) and holds the
// bookkeeping methods charge and refund settle their own retries and
// follow-ups through; Dispatcher, a separate value built after charge and
// refund exist, satisfies platform/jobs.Processor. Splitting the two is what
// keeps Command free of a cycle back into charge/refund -- see dispatcher.go.
package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error) {
	return c.repo.Claim(ctx, batchSize, leaseDuration)
}

func (c *Command) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	return c.repo.Prune(ctx, olderThan, limit)
}

// CancelPendingByOrderID satisfies order/cancel's PaymentJobCanceller and is
// also called from webhook's failure path.
func (c *Command) CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return c.repo.CancelJobsByOrderID(ctx, orderID)
}

// MarkJobCompleted lets charge and refund settle the real job row a worker
// tick claimed; InitiatePayment's and webhook's synthetic jobs pass
// uuid.Nil, which matches zero rows and is a deliberate no-op.
func (c *Command) MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error {
	return c.repo.MarkJobCompleted(ctx, jobID)
}

// MarkJobCompletedByPaymentID backs webhook's success path: a webhook can
// race a worker tick that already claimed the job, so completion is keyed on
// the payment rather than a job id the webhook never had.
func (c *Command) MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error {
	return c.repo.MarkJobCompletedByPaymentID(ctx, paymentID, action)
}

// UpdateJob lets charge and refund persist their own retry/backoff bookkeeping
// on the job row jobs owns.
func (c *Command) UpdateJob(ctx context.Context, job *domain.Job) error {
	return c.repo.UpdateJob(ctx, job)
}

// EnqueueRefund backs refund's admin-triggered Execute, and charge's own
// FinalizePaymentSuccess/RunCompensatingRefund when a charge needs a
// follow-up refund.
func (c *Command) EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	return c.repo.CreateJob(ctx, &domain.Job{
		PaymentID:   paymentID,
		OrderID:     orderID,
		Action:      domain.ActionRefund,
		Status:      domain.JobStatusPending,
		NextRetryAt: time.Now(),
	})
}
