// Package jobs owns the payment_jobs queue table and drains it: Claim and
// Prune satisfy platform/jobs.Queue, Process satisfies platform/jobs.Processor,
// and cmd/worker runs one value as both. Process only dispatches -- charge and
// refund own the mechanics of a charge or refund attempt, including the
// finalize/compensate dance, because that logic decides order and inventory
// state that belongs to their own slice, not to the queue that merely
// schedules it.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type Command struct {
	repo   Repository
	charge ChargeProcessor
	refund RefundProcessor
	logger *slog.Logger
}

func New(repo Repository, log *slog.Logger) *Command {
	return &Command{repo: repo, logger: log}
}

// SetProcessors breaks the jobs/charge/refund construction cycle: charge and
// refund each need jobs (to enqueue a follow-up job or complete their own),
// and jobs needs both of them back to dispatch a claimed job, so
// payment/module.go builds jobs first and wires this in once charge and
// refund exist too.
func (c *Command) SetProcessors(charge ChargeProcessor, refund RefundProcessor) {
	c.charge = charge
	c.refund = refund
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

// Process owns no retry logic of its own: it dispatches to whichever slice
// owns the action, and that slice's own bookkeeping decides the backoff.
func (c *Command) Process(ctx context.Context, job domain.Job) error {
	// Only here: InitiatePayment and the webhook reach the same finalize path
	// with a synthetic Job that has no ID, so they must not set this.
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	switch job.Action {
	case domain.ActionCharge:
		return c.charge.ProcessCharge(ctx, job)
	case domain.ActionRefund:
		return c.refund.ProcessRefund(ctx, job)
	default:
		c.logger.ErrorContext(ctx, "unknown job action", slog.String("action", string(job.Action)))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}
