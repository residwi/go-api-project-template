package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
)

type ExpireStaleArgs struct{}

func (ExpireStaleArgs) Kind() string { return "order.expire-stale" }

func (ExpireStaleArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "order",
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByQueue: true},
	}
}

type StaleSweeper interface {
	RecoverStale(ctx context.Context) error
	ExpireStale(ctx context.Context) error
}

type ExpireStaleWorker struct {
	river.WorkerDefaults[ExpireStaleArgs]

	service StaleSweeper
	logger  *slog.Logger
	timeout time.Duration
}

func NewExpireStaleWorker(service StaleSweeper, logger *slog.Logger, timeout time.Duration) *ExpireStaleWorker {
	return &ExpireStaleWorker{service: service, logger: logger, timeout: timeout}
}

func (w *ExpireStaleWorker) Timeout(*river.Job[ExpireStaleArgs]) time.Duration { return w.timeout }

func (w *ExpireStaleWorker) Work(ctx context.Context, _ *river.Job[ExpireStaleArgs]) error {
	if err := w.service.RecoverStale(ctx); err != nil {
		w.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}

	return w.service.ExpireStale(ctx)
}
