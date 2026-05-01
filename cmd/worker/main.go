package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Setup(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Repositories
	orderRepo := order.NewPostgresRepository(pool)
	paymentRepo := payment.NewPostgresRepository(pool)
	inventoryRepo := inventory.NewPostgresRepository(pool)
	promotionRepo := promotion.NewPostgresRepository(pool)
	notificationRepo := notification.NewPostgresRepository(pool)

	// Services
	inventorySvc := inventory.NewService(inventoryRepo, pool)
	promotionSvc := promotion.NewService(promotionRepo, pool)
	notificationSvc := notification.NewService(notificationRepo)

	// Order service (no payment deps needed for worker — it only reads orders)
	orderSvc := order.NewService(
		orderRepo, pool,
		nil, // cart — not used by worker
		nil, // inventory reserver — not used by worker
		nil, // payment initiator — not used by worker
		nil, // payment canceller — not used by worker
		nil, // coupon reserver — not used by worker
		nil, // notification enqueuer — not used by worker
	)

	// Payment gateway
	gw := mockgw.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	// Payment service
	paymentSvc := payment.NewService(
		paymentRepo, pool, gw,
		&workerOrderUpdaterAdapter{svc: orderSvc},
		&workerOrderGetterAdapter{svc: orderSvc},
		&workerOrderItemsGetterAdapter{svc: orderSvc},
		&workerInventoryDeductorAdapter{svc: inventorySvc},
		&workerInventoryReleaserAdapter{svc: inventorySvc},
		&workerInventoryRestockerAdapter{svc: inventorySvc},
		&workerCouponReleaserAdapter{svc: promotionSvc},
	)

	// Worker
	_ = notificationSvc // available for future notification job processing

	w := payment.NewWorker(
		paymentRepo, pool, paymentSvc,
		&workerOrderUpdaterAdapter{svc: orderSvc},
		&workerOrderItemsGetterAdapter{svc: orderSvc},
		&workerOrderGetterAdapter{svc: orderSvc},
		&workerInventoryReleaserAdapter{svc: inventorySvc},
		&workerCouponReleaserAdapter{svc: promotionSvc},
		payment.WorkerConfig{
			Interval:      cfg.Worker.Interval,
			BatchSize:     cfg.Worker.BatchSize,
			LeaseDuration: cfg.Worker.LeaseDuration,
			Concurrency:   cfg.Worker.Concurrency,
		},
	)

	slog.Info("worker starting", "env", cfg.App.Env)
	w.Start(ctx)
	slog.Info("worker stopped")
	return nil
}

// ── Adapters (same pattern as server/router.go) ──────────────────────────

type workerOrderUpdaterAdapter struct {
	svc *order.Service
}

func (a *workerOrderUpdaterAdapter) UpdateStatus(ctx context.Context, orderID uuid.UUID, fromStatuses []string, toStatus string) error {
	from := make([]order.Status, len(fromStatuses))
	for i, s := range fromStatuses {
		from[i] = order.Status(s)
	}
	return a.svc.UpdateStatusMulti(ctx, orderID, from, order.Status(toStatus))
}

type workerOrderGetterAdapter struct {
	svc *order.Service
}

func (a *workerOrderGetterAdapter) GetByID(ctx context.Context, orderID uuid.UUID) (payment.OrderSnapshot, error) {
	o, err := a.svc.AdminGetByID(ctx, orderID)
	if err != nil {
		return payment.OrderSnapshot{}, err
	}
	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}
	return payment.OrderSnapshot{
		TotalAmount: o.TotalAmount,
		Currency:    o.Currency,
		Status:      string(o.Status),
		CouponCode:  couponCode,
	}, nil
}

type workerOrderItemsGetterAdapter struct {
	svc *order.Service
}

func (a *workerOrderItemsGetterAdapter) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]payment.OrderItemDTO, error) {
	items, err := a.svc.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	result := make([]payment.OrderItemDTO, len(items))
	for i, item := range items {
		result[i] = payment.OrderItemDTO{ProductID: item.ProductID, Quantity: item.Quantity}
	}
	return result, nil
}

type workerInventoryDeductorAdapter struct {
	svc *inventory.Service
}

func (a *workerInventoryDeductorAdapter) Deduct(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Deduct(ctx, productID, qty)
	return err
}

type workerInventoryReleaserAdapter struct {
	svc *inventory.Service
}

func (a *workerInventoryReleaserAdapter) Release(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Release(ctx, productID, qty)
	return err
}

type workerInventoryRestockerAdapter struct {
	svc *inventory.Service
}

func (a *workerInventoryRestockerAdapter) Restock(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := a.svc.Restock(ctx, productID, qty)
	return err
}

type workerCouponReleaserAdapter struct {
	svc *promotion.Service
}

func (a *workerCouponReleaserAdapter) Release(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Release(ctx, orderID)
}
