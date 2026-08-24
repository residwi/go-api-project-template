package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type LegacyQueue[T any] interface {
	Claim(ctx context.Context, batch int, lease time.Duration) ([]T, error)
	Prune(ctx context.Context, age time.Duration, limit int) (int, error)
}

type LegacyProcessor[T any] interface {
	Process(ctx context.Context, job T) error
}

type Sweeper interface {
	Sweep(ctx context.Context) error
}

type Config struct {
	Interval      time.Duration
	BatchSize     int
	LeaseDuration time.Duration
	Concurrency   int
	PruneAge      time.Duration
	PruneLimit    int
}

type LegacyRunner[T any] struct {
	name   string
	queue  LegacyQueue[T]
	proc   LegacyProcessor[T]
	cfg    Config
	logger *slog.Logger
}

// Bounds a job's timeout to 4/5 of its lease. Without the margin, cancellation
// and the reclaim window coincide and two workers run the same job.
const leaseSafetyDivisor = 5

func NewLegacyRunner[T any](name string, queue LegacyQueue[T], proc LegacyProcessor[T], cfg Config, log *slog.Logger) *LegacyRunner[T] {
	return &LegacyRunner[T]{name: name, queue: queue, proc: proc, cfg: cfg, logger: log}
}

func (r *LegacyRunner[T]) Start(ctx context.Context) {
	ctx = logger.WithAttrs(ctx, slog.String("runner", r.name))

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *LegacyRunner[T]) tick(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.ErrorContext(ctx, "tick panicked", slog.String("panic", fmt.Sprint(rec)))
		}
	}()

	if sweeper, ok := r.proc.(Sweeper); ok {
		if err := sweeper.Sweep(ctx); err != nil {
			r.logger.ErrorContext(ctx, "sweep failed", slog.String("error", err.Error()))
		}
	}

	if _, err := r.queue.Prune(ctx, r.cfg.PruneAge, r.cfg.PruneLimit); err != nil {
		r.logger.ErrorContext(ctx, "prune jobs failed", slog.String("error", err.Error()))
	}

	batch, err := r.queue.Claim(ctx, r.cfg.BatchSize, r.cfg.LeaseDuration)
	if err != nil {
		r.logger.ErrorContext(ctx, "claim jobs failed", slog.String("error", err.Error()))
		return
	}

	deadline := time.Now().Add(r.cfg.LeaseDuration - r.cfg.LeaseDuration/leaseSafetyDivisor)

	var wg sync.WaitGroup
	sem := make(chan struct{}, r.cfg.Concurrency)
	for _, job := range batch {
		wg.Add(1)
		sem <- struct{}{}
		go func(job T) {
			defer wg.Done()
			defer func() { <-sem }()
			r.processOne(ctx, job, deadline)
		}(job)
	}
	wg.Wait()
}

func (r *LegacyRunner[T]) processOne(ctx context.Context, job T, deadline time.Time) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.ErrorContext(ctx, "job panicked", slog.String("panic", fmt.Sprint(rec)))
		}
	}()
	jobCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := r.proc.Process(jobCtx, job); err != nil {
		r.logger.WarnContext(ctx, "job did not complete", slog.String("error", err.Error()))
	}
}
