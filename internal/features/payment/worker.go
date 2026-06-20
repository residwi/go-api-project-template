package payment

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Worker struct {
	repo    Repository
	pool    *pgxpool.Pool
	service *Service
	cfg     WorkerConfig
}

type WorkerConfig struct {
	Interval      time.Duration
	BatchSize     int
	LeaseDuration time.Duration
	Concurrency   int
}

// leaseSafetyDivisor bounds a job's processing timeout to
// lease - lease/leaseSafetyDivisor (4/5 of the lease), leaving a margin before
// the job's claim expires and it becomes reclaimable by another worker.
const leaseSafetyDivisor = 5

func NewWorker(repo Repository, pool *pgxpool.Pool, service *Service, cfg WorkerConfig) *Worker {
	return &Worker{repo: repo, pool: pool, service: service, cfg: cfg}
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
			// Bound each job to a fraction of the lease (80%) so it aborts BEFORE
			// the lease expires and the job becomes reclaimable — otherwise the
			// cancellation and the reclaim window coincide, letting two workers
			// run the same job at the boundary.
			jobCtx, cancel := context.WithTimeout(ctx, w.cfg.LeaseDuration-w.cfg.LeaseDuration/leaseSafetyDivisor)
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
	// Expiring an order must also release the stock and coupon usage it reserved
	// at checkout, otherwise that reserved inventory/coupon usage leaks forever.
	// Done in one transaction so the expiry and the releases commit together.
	err := database.WithTx(ctx, w.pool, func(txCtx context.Context) error {
		db := database.DB(txCtx, w.pool)

		// Expire a bounded batch of stale awaiting_payment orders, locking the rows
		// so concurrent workers don't double-process them.
		rows, err := db.Query(txCtx,
			`UPDATE orders SET status = 'expired'
			 WHERE id IN (
			     SELECT id FROM orders
			     WHERE status = 'awaiting_payment' AND created_at < NOW() - INTERVAL '30 minutes'
			     ORDER BY created_at
			     LIMIT 20
			     FOR UPDATE SKIP LOCKED
			 )
			 RETURNING id`)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, scanOrderID)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		// Release reserved stock for the expired orders' items.
		if _, err := db.Exec(txCtx,
			`UPDATE products p
			 SET reserved_quantity = GREATEST(p.reserved_quantity - agg.qty, 0)
			 FROM (
			     SELECT product_id, SUM(quantity) AS qty
			     FROM order_items WHERE order_id = ANY($1)
			     GROUP BY product_id
			 ) agg
			 WHERE p.id = agg.product_id`, ids); err != nil {
			return err
		}

		// Release coupon reservations: drop the usage rows and decrement counts.
		if _, err := db.Exec(txCtx,
			`WITH freed AS (
			     DELETE FROM coupon_usages WHERE order_id = ANY($1) RETURNING coupon_id
			 )
			 UPDATE promotions pr
			 SET used_count = GREATEST(pr.used_count - cnt.n, 0)
			 FROM (SELECT coupon_id, COUNT(*) AS n FROM freed GROUP BY coupon_id) cnt
			 WHERE pr.id = cnt.coupon_id`, ids); err != nil {
			return err
		}

		slog.InfoContext(ctx, "swept expired orders", "count", len(ids))
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "sweep expired orders failed", "error", err)
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

func scanOrderID(row pgx.CollectableRow) (uuid.UUID, error) {
	var id uuid.UUID
	err := row.Scan(&id)
	return id, err
}
