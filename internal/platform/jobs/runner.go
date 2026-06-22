package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Queue is a feature's job persistence: claim a leased batch to work, and prune
// finished rows so the table stays bounded.
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

// Config tunes the runner's loop.
type Config struct {
	Interval      time.Duration
	BatchSize     int
	LeaseDuration time.Duration
	Concurrency   int
	PruneAge      time.Duration
	PruneLimit    int
}

// Runner drives a Queue and a Processor on an interval.
type Runner[T any] struct {
	name  string
	queue Queue[T]
	proc  Processor[T]
	cfg   Config
}

// leaseSafetyDivisor bounds a job's processing timeout to lease - lease/divisor
// (4/5 of the lease), leaving a margin before the claim expires and the job
// becomes reclaimable — otherwise the cancellation and the reclaim window
// coincide, letting two workers run the same job at the boundary.
const leaseSafetyDivisor = 5

func NewRunner[T any](name string, queue Queue[T], proc Processor[T], cfg Config) *Runner[T] {
	return &Runner[T]{name: name, queue: queue, proc: proc, cfg: cfg}
}

// Start runs the loop until ctx is cancelled.
func (r *Runner[T]) Start(ctx context.Context) {
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
	if sweeper, ok := r.proc.(Sweeper); ok {
		if err := sweeper.Sweep(ctx); err != nil {
			slog.ErrorContext(ctx, "sweep failed", "runner", r.name, "error", err)
		}
	}

	if _, err := r.queue.Prune(ctx, r.cfg.PruneAge, r.cfg.PruneLimit); err != nil {
		slog.ErrorContext(ctx, "prune jobs failed", "runner", r.name, "error", err)
	}

	batch, err := r.queue.Claim(ctx, r.cfg.BatchSize, r.cfg.LeaseDuration)
	if err != nil {
		slog.ErrorContext(ctx, "claim jobs failed", "runner", r.name, "error", err)
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, r.cfg.Concurrency)
	for _, job := range batch {
		wg.Add(1)
		sem <- struct{}{}
		go func(job T) {
			defer wg.Done()
			defer func() { <-sem }()
			r.processOne(ctx, job)
		}(job)
	}
	wg.Wait()
}

func (r *Runner[T]) processOne(ctx context.Context, job T) {
	timeout := r.cfg.LeaseDuration - r.cfg.LeaseDuration/leaseSafetyDivisor
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := r.proc.Process(jobCtx, job); err != nil {
		slog.WarnContext(ctx, "job did not complete", "runner", r.name, "error", err)
	}
}
