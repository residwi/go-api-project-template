package worker

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/jobs"
)

type OrderHousekeeper interface {
	ExpireStale(ctx context.Context) error
	RecoverStaleProcessing(ctx context.Context) error
}

type Processor struct {
	*jobs.Dispatcher

	orders OrderHousekeeper
	logger *slog.Logger
}

func NewProcessor(dispatcher *jobs.Dispatcher, orders OrderHousekeeper, log *slog.Logger) *Processor {
	return &Processor{Dispatcher: dispatcher, orders: orders, logger: log}
}

func (p *Processor) Sweep(ctx context.Context) error {
	if err := p.orders.RecoverStaleProcessing(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return p.orders.ExpireStale(ctx)
}
