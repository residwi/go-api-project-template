package payment

import "context"

// OrderExpirer expires stale awaiting_payment orders. It's an inline cross-feature
// interface (like OrderUpdater/OrderGetter); the order module owns the logic and
// the wiring adapter supplies it.
type OrderExpirer interface {
	ExpireStale(ctx context.Context) error
}

// JobProcessor is what the payment job runner drains payment_jobs with: it
// processes each job via the embedded Service, and as the runner's Sweep hook it
// delegates per-tick housekeeping (expiring stale orders) to the order module —
// so payment never reaches into orders/inventory/coupons itself.
type JobProcessor struct {
	*Service

	orders OrderExpirer
}

func NewJobProcessor(svc *Service, orders OrderExpirer) *JobProcessor {
	return &JobProcessor{Service: svc, orders: orders}
}

// Sweep is the job runner's optional per-tick housekeeping hook.
func (p *JobProcessor) Sweep(ctx context.Context) error {
	return p.orders.ExpireStale(ctx)
}
