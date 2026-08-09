// Package worker adapts jobs.Command into the worker's per-tick Sweep hook.
// It exists because jobs owns only the queue; expiring stale orders and
// recovering ones stuck in payment_processing are order's own housekeeping,
// which rides payment's queue tick rather than getting a runner of its own --
// that coupling predates this module's slicing and is kept deliberately: it
// is a behaviour decision, not a boundary one, and changing which process's
// tick drives order's recovery sweep is out of scope for a move.
package worker

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/jobs"
)

// OrderHousekeeper is owned by the order module -- expiring stale orders,
// recovering ones stuck in payment_processing -- and supplied by bootstrap.
type OrderHousekeeper interface {
	ExpireStale(ctx context.Context) error
	RecoverStaleProcessing(ctx context.Context) error
}

type Processor struct {
	*jobs.Command

	orders OrderHousekeeper
	// logger is its own field rather than the embedded Command's: that one is
	// unexported and so unreachable from this package.
	logger *slog.Logger
}

func NewProcessor(cmd *jobs.Command, orders OrderHousekeeper, log *slog.Logger) *Processor {
	return &Processor{Command: cmd, orders: orders, logger: log}
}

// Sweep is the runner's optional per-tick hook. A recovery failure is logged, not
// returned, so it never blocks the expiry half.
func (p *Processor) Sweep(ctx context.Context) error {
	if err := p.orders.RecoverStaleProcessing(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return p.orders.ExpireStale(ctx)
}
