package payment

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	repo           Repository
	pool           *pgxpool.Pool
	service        *Service
	orderUpdater   OrderUpdater
	orderItems     OrderItemsGetter
	orderGet       OrderGetter
	inventoryRel   InventoryReleaser
	couponReleaser CouponReleaser
	cfg            WorkerConfig
}

type WorkerConfig struct {
	Interval      time.Duration
	BatchSize     int
	LeaseDuration time.Duration
	Concurrency   int
}

func NewWorker(
	repo Repository,
	pool *pgxpool.Pool,
	service *Service,
	orderUpdater OrderUpdater,
	orderItems OrderItemsGetter,
	orderGet OrderGetter,
	inventoryRel InventoryReleaser,
	couponReleaser CouponReleaser,
	cfg WorkerConfig,
) *Worker {
	return &Worker{
		repo:           repo,
		pool:           pool,
		service:        service,
		orderUpdater:   orderUpdater,
		orderItems:     orderItems,
		orderGet:       orderGet,
		inventoryRel:   inventoryRel,
		couponReleaser: couponReleaser,
		cfg:            cfg,
	}
}

func (w *Worker) Start(ctx context.Context) {
	slog.InfoContext(ctx, "payment worker starting",
		"interval", w.cfg.Interval,
		"batch_size", w.cfg.BatchSize,
		"concurrency", w.cfg.Concurrency)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "payment worker stopping")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	// Sweep and cleanup are fast single-statement maintenance queries; run them
	// before the (potentially slow, gateway-bound) job processing so order expiry
	// and job cleanup are never starved by a slow batch of charge jobs.
	w.sweepExpiredOrders(ctx)
	w.cleanupOldJobs(ctx)
	w.processJobs(ctx)
}

func (w *Worker) processJobs(ctx context.Context) {
	jobs, err := w.repo.ClaimPendingJobs(ctx, w.cfg.BatchSize, w.cfg.LeaseDuration)
	if err != nil {
		slog.ErrorContext(ctx, "failed to claim payment jobs", "error", err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	slog.InfoContext(ctx, "processing payment jobs", "count", len(jobs))

	var wg sync.WaitGroup
	sem := make(chan struct{}, w.cfg.Concurrency)

	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			// Bound each job to the lease duration so a hung gateway call can't
			// pin a goroutine (and its pooled connection) past the point where
			// the job's claim would expire and be re-claimed anyway.
			jobCtx, cancel := context.WithTimeout(ctx, w.cfg.LeaseDuration)
			defer cancel()
			if ok := w.service.ProcessJob(jobCtx, j); !ok {
				slog.WarnContext(ctx, "payment job did not complete successfully",
					"job_id", j.ID, "order_id", j.OrderID)
			}
		}(job)
	}

	wg.Wait()
}

func (w *Worker) sweepExpiredOrders(ctx context.Context) {
	// Expire stale awaiting_payment orders in a single UPDATE instead of a SELECT
	// followed by one UPDATE per row. The LIMIT (via subselect) bounds the batch.
	tag, err := w.pool.Exec(ctx,
		`UPDATE orders SET status = 'expired'
		 WHERE id IN (
		     SELECT id FROM orders
		     WHERE status = 'awaiting_payment' AND created_at < NOW() - INTERVAL '30 minutes'
		     ORDER BY created_at
		     LIMIT 20
		 )`)
	if err != nil {
		slog.ErrorContext(ctx, "sweep expired orders: update failed", "error", err)
		return
	}

	if count := tag.RowsAffected(); count > 0 {
		slog.InfoContext(ctx, "swept expired orders", "count", count)
	}
}

func (w *Worker) cleanupOldJobs(ctx context.Context) {
	deleted, err := w.repo.DeleteOldCompletedJobs(ctx, 7*24*time.Hour, 100)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup old payment jobs failed", "error", err)
		return
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "cleaned up old payment jobs", "deleted", deleted)
	}
}
