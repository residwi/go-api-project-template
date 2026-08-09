package worker

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

type Processor struct {
	*payment.Service

	orders payment.OrderHousekeeper
	// logger is its own field rather than the embedded Service's: that one is
	// unexported and so unreachable from this package.
	logger *slog.Logger
}

func NewProcessor(svc *payment.Service, orders payment.OrderHousekeeper, log *slog.Logger) *Processor {
	return &Processor{Service: svc, orders: orders, logger: log}
}

// Sweep is the runner's optional per-tick hook. A recovery failure is logged, not
// returned, so it never blocks the expiry half.
func (p *Processor) Sweep(ctx context.Context) error {
	if err := p.orders.RecoverStaleProcessing(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return p.orders.ExpireStale(ctx)
}
