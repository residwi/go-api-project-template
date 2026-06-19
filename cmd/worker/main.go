package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
	"github.com/residwi/go-api-project-template/internal/wiring"
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

	orderRepo := order.NewPostgresRepository(pool)
	paymentRepo := payment.NewPostgresRepository(pool)
	inventoryRepo := inventory.NewPostgresRepository(pool)
	promotionRepo := promotion.NewPostgresRepository(pool)
	notificationRepo := notification.NewPostgresRepository(pool)

	inventorySvc := inventory.NewService(inventoryRepo, pool)
	promotionSvc := promotion.NewService(promotionRepo, pool)
	notificationSvc := notification.NewService(notificationRepo)

	// The worker only reads orders, so order's cross-feature deps are all nil.
	orderSvc := order.NewService(orderRepo, pool, nil, nil, nil, nil, nil, nil)

	gw := mockgw.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	paymentSvc := wiring.NewPaymentService(paymentRepo, pool, gw, orderSvc, inventorySvc, promotionSvc)

	_ = notificationSvc // available for future notification job processing

	w := wiring.NewPaymentWorker(
		paymentRepo, pool, paymentSvc,
		orderSvc, inventorySvc, promotionSvc,
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
