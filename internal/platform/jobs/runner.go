package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type Processor interface {
	Process(ctx context.Context, rec Record) error
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

type Runner struct {
	queue  string
	store  Store
	proc   Processor
	cfg    Config
	logger *slog.Logger
}

// Bounds a job's timeout to 4/5 of its lease. Without the margin, cancellation
// and the reclaim window coincide and two workers run the same job.
const leaseSafetyDivisor = 5

const (
	backoffJitterDivisor = 2
	backoffMaxShift      = 30
)

func NewRunner(queue string, store Store, proc Processor, cfg Config, log *slog.Logger) *Runner {
	return &Runner{queue: queue, store: store, proc: proc, cfg: cfg, logger: log}
}

func (r *Runner) Start(ctx context.Context) {
	ctx = logger.WithAttrs(ctx, slog.String("runner", r.queue))

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

func (r *Runner) tick(ctx context.Context) {
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

	if _, err := r.store.Prune(ctx, r.queue, r.cfg.PruneAge, r.cfg.PruneLimit); err != nil {
		r.logger.ErrorContext(ctx, "prune jobs failed", slog.String("error", err.Error()))
	}

	batch, err := r.store.Claim(ctx, r.queue, r.cfg.BatchSize, r.cfg.LeaseDuration)
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
		go func(rec Record) {
			defer wg.Done()
			defer func() { <-sem }()
			r.processOne(ctx, rec, deadline)
		}(job)
	}
	wg.Wait()
}

func (r *Runner) processOne(ctx context.Context, rec Record, deadline time.Time) {
	ctx = logger.WithAttrs(ctx, slog.String("job_id", rec.ID.String()))

	err := r.run(ctx, rec, deadline)
	r.settle(ctx, rec, err)
}

func (r *Runner) run(ctx context.Context, rec Record, deadline time.Time) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			r.logger.ErrorContext(ctx, "job panicked", slog.String("panic", fmt.Sprint(recovered)))
			err = fmt.Errorf("job panicked: %v", recovered)
		}
	}()

	jobCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	return r.proc.Process(jobCtx, rec)
}

func (r *Runner) settle(ctx context.Context, rec Record, procErr error) {
	if procErr == nil {
		if err := r.store.Complete(ctx, rec.ID); err != nil {
			r.logger.ErrorContext(ctx, "marking job complete failed", slog.String("error", err.Error()))
		}
		return
	}

	if errors.Is(procErr, ErrDiscard) {
		if err := r.store.Cancel(ctx, rec.ID, procErr.Error()); err != nil {
			r.logger.ErrorContext(ctx, "cancelling job failed", slog.String("error", err.Error()))
		}
		return
	}

	attempts := rec.Attempts + 1
	if attempts >= rec.MaxAttempts {
		r.logger.WarnContext(ctx, "job exhausted its retries", slog.String("error", procErr.Error()))
		if err := r.store.Bury(ctx, rec.ID, attempts, procErr.Error()); err != nil {
			r.logger.ErrorContext(ctx, "burying job failed", slog.String("error", err.Error()))
		}
		return
	}

	if err := r.store.Retry(ctx, rec.ID, attempts, procErr.Error(), time.Now().Add(backoff(attempts))); err != nil {
		r.logger.ErrorContext(ctx, "scheduling job retry failed", slog.String("error", err.Error()))
	}
}

func backoff(attempts int) time.Duration {
	base := time.Duration(1<<min(max(attempts, 0), backoffMaxShift)) * time.Second
	jitter := time.Duration(
		rand.N(int64(base / backoffJitterDivisor)), //nolint:gosec // jitter needs no crypto randomness
	)
	return base + jitter
}
