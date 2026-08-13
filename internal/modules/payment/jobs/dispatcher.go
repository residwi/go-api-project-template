package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

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
		return d.charge.ProcessCharge(ctx, job)
	case domain.ActionRefund:
		return d.refund.ProcessRefund(ctx, job)
	default:
		d.logger.ErrorContext(ctx, "unknown job action", slog.String("action", string(job.Action)))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}
