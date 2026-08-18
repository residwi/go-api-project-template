package worker

import (
	"context"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/payment/jobs"
)

type StaleExpirer interface {
	ExpireStale(ctx context.Context) error
}

type StaleRecoverer interface {
	RecoverStaleProcessing(ctx context.Context) error
}

type Deps struct {
	Dispatcher *jobs.Dispatcher
	Expirer    StaleExpirer
	Recoverer  StaleRecoverer
	Logger     *slog.Logger
}

type Processor struct {
	*jobs.Dispatcher

	expirer   StaleExpirer
	recoverer StaleRecoverer
	logger    *slog.Logger
}

func NewProcessor(d Deps) *Processor {
	return &Processor{
		Dispatcher: d.Dispatcher,
		expirer:    d.Expirer,
		recoverer:  d.Recoverer,
		logger:     d.Logger,
	}
}

func (p *Processor) Sweep(ctx context.Context) error {
	if err := p.recoverer.RecoverStaleProcessing(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return p.expirer.ExpireStale(ctx)
}
