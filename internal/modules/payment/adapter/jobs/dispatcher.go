// Package jobs holds the dispatcher that routes a claimed payment job to the
// charge or refund path. It declares its own narrow ports rather than
// importing the payment module root: the root package constructs a Queue
// from Deps.Pool (see payment/jobs.go), so an import running the other way
// -- this package back into payment -- would cycle.
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type ChargeProcessor interface {
	RunChargeJob(ctx context.Context, job domain.Job) error
}

type RefundProcessor interface {
	RunRefundJob(ctx context.Context, job domain.Job) error
}

type Dispatcher struct {
	charge ChargeProcessor
	refund RefundProcessor
	logger *slog.Logger
}

func NewDispatcher(charge ChargeProcessor, refund RefundProcessor, log *slog.Logger) *Dispatcher {
	return &Dispatcher{charge: charge, refund: refund, logger: log}
}

func (d *Dispatcher) Process(ctx context.Context, job domain.Job) error {
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	switch job.Action {
	case domain.ActionCharge:
		return d.charge.RunChargeJob(ctx, job)
	case domain.ActionRefund:
		return d.refund.RunRefundJob(ctx, job)
	default:
		d.logger.ErrorContext(ctx, "unknown job action", slog.String("action", string(job.Action)))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}
