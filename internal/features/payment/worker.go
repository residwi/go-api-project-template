package payment

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
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
	w.processJobs(ctx)
	w.sweepExpiredOrders(ctx)
	w.cleanupOldJobs(ctx)
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
			w.service.ProcessJob(ctx, j)
		}(job)
	}

	wg.Wait()
}

func (w *Worker) sweepExpiredOrders(ctx context.Context) {
	rows, err := w.pool.Query(ctx,
		`SELECT id FROM orders
		 WHERE status = 'awaiting_payment' AND created_at < NOW() - INTERVAL '30 minutes'
		 ORDER BY created_at LIMIT 20`)
	if err != nil {
		slog.ErrorContext(ctx, "sweep expired orders: query failed", "error", err)
		return
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var orderID uuid.UUID
		if err := rows.Scan(&orderID); err != nil {
			continue
		}
		err := w.orderUpdater.UpdateStatus(ctx, orderID,
			[]string{"awaiting_payment"}, "expired")
		if err != nil {
			slog.WarnContext(ctx, "sweep expired: failed to expire order",
				"order_id", orderID, "error", err)
			continue
		}
		count++
	}

	if count > 0 {
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
