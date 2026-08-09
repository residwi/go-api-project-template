package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

// Dispatcher satisfies platform/jobs.Processor by routing a claimed job to
// whichever slice owns its action. It is a value separate from Command on
// purpose: charge and refund each need Command back, to enqueue a follow-up
// job or settle their own retry bookkeeping, so if Command also dispatched
// to charge and refund it would need them back too -- the same three-way
// cycle one level up, forced onto a setter exported on Command. Command is
// reachable as order/cancel's PaymentJobCanceller and as the worker's queue
// argument, so that setter would compile from cmd/, from internal/transport/,
// from any test -- and a call passing nil would panic the worker on the next
// claimed job. Dispatcher is built once, after charge and refund both exist,
// and nothing outside payment/module.go ever sees it.
type Dispatcher struct {
	charge ChargeProcessor
	refund RefundProcessor
	logger *slog.Logger
}

func NewDispatcher(charge ChargeProcessor, refund RefundProcessor, log *slog.Logger) *Dispatcher {
	return &Dispatcher{charge: charge, refund: refund, logger: log}
}

// Process owns no retry logic of its own: it dispatches to whichever slice
// owns the action, and that slice's own bookkeeping decides the backoff.
func (d *Dispatcher) Process(ctx context.Context, job domain.Job) error {
	// Only here: InitiatePayment and the webhook reach the same finalize path
	// with a synthetic Job that has no ID, so they must not set this.
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	switch job.Action {
	case domain.ActionCharge:
		return d.charge.ProcessCharge(ctx, job)
	case domain.ActionRefund:
		return d.refund.ProcessRefund(ctx, job)
	default:
		d.logger.ErrorContext(ctx, "unknown job action", slog.String("action", string(job.Action)))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}
