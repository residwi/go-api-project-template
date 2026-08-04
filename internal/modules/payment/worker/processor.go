package worker

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

// Processor is what the payment job runner drains payment_jobs with: it
// processes each job via the embedded Service, and as the runner's Sweep hook it
// delegates per-tick housekeeping (expiring stale orders) to the order module —
// so payment never reaches into orders/inventory/coupons itself.
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

// Sweep is the job runner's optional per-tick housekeeping hook. It recovers
// orders stuck in payment_processing (a worker that died mid-charge) and expires
// stale awaiting_payment orders. Recovery failures are logged, not fatal, so a
// recovery hiccup never blocks expiry.
func (p *Processor) Sweep(ctx context.Context) error {
	if err := p.orders.RecoverStaleProcessing(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.Any("error", err))
	}
	return p.orders.ExpireStale(ctx)
}
