package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type Queue[T any] interface {
	Claim(ctx context.Context, batch int, lease time.Duration) ([]T, error)
	Prune(ctx context.Context, age time.Duration, limit int) (int, error)
}

// Processor does the per-job work and owns its own retry/backoff bookkeeping.
type Processor[T any] interface {
	Process(ctx context.Context, job T) error
}

// Sweeper is an optional capability a Processor may implement to run
// feature-specific housekeeping once per tick (e.g. expiring stale records).
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

type Runner[T any] struct {
	name   string
	queue  Queue[T]
	proc   Processor[T]
	cfg    Config
	logger *slog.Logger
}

// Bounds a job's timeout to 4/5 of its lease. Without the margin, cancellation
// and the reclaim window coincide and two workers run the same job.
const leaseSafetyDivisor = 5

func NewRunner[T any](name string, queue Queue[T], proc Processor[T], cfg Config, log *slog.Logger) *Runner[T] {
	return &Runner[T]{name: name, queue: queue, proc: proc, cfg: cfg, logger: log}
}

// Start runs the loop until ctx is cancelled.
func (r *Runner[T]) Start(ctx context.Context) {
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

func (r *Runner[T]) tick(ctx context.Context) {
	// Sweep and Prune run on the loop goroutine, so a panic here would take the
	// runner down -- and every other runner in the worker binary with it.
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

	// The lease starts at claim time for the whole batch, so every deadline is
	// measured from now, not from when a goroutine happens to start: with
	// BatchSize > Concurrency a late starter would otherwise outlive the lease and
	// be reclaimed by another worker while still running.
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

func (r *Runner[T]) processOne(ctx context.Context, job T, deadline time.Time) {
	// A Process panic must not take down the worker; isolate it to this job.
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
