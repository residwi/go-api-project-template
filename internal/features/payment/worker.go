package payment

import (
	"context"
	"log/slog"
)

// OrderHousekeeper runs the order module's per-tick housekeeping: expiring stale
// awaiting_payment orders and recovering orders stuck in payment_processing
// (e.g. after a worker died mid-charge). It's an inline cross-feature interface
// (like OrderUpdater/OrderGetter); the order module owns the logic and the
// wiring adapter supplies it.
type OrderHousekeeper interface {
	ExpireStale(ctx context.Context) error
	RecoverStaleProcessing(ctx context.Context) error
}

// JobProcessor is what the payment job runner drains payment_jobs with: it
// processes each job via the embedded Service, and as the runner's Sweep hook it
// delegates per-tick housekeeping (expiring stale orders) to the order module —
// so payment never reaches into orders/inventory/coupons itself.
type JobProcessor struct {
	*Service

	orders OrderHousekeeper
}

func NewJobProcessor(svc *Service, orders OrderHousekeeper) *JobProcessor {
	return &JobProcessor{Service: svc, orders: orders}
}

// Sweep is the job runner's optional per-tick housekeeping hook. It recovers
// orders stuck in payment_processing (a worker that died mid-charge) and expires
// stale awaiting_payment orders. Recovery failures are logged, not fatal, so a
// recovery hiccup never blocks expiry.
func (p *JobProcessor) Sweep(ctx context.Context) error {
	if err := p.orders.RecoverStaleProcessing(ctx); err != nil {
		slog.ErrorContext(ctx, "recover stale processing orders failed", "error", err)
	}
	return p.orders.ExpireStale(ctx)
}
