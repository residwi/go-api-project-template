package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	paymentworker "github.com/residwi/go-api-project-template/internal/modules/payment/worker"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		logger.FromEnv().Error("loading config failed", slog.String("error", err.Error()))
		return fmt.Errorf("loading config: %w", err)
	}

	appLog := logger.Setup(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		// Reported as well as returned: main only sets the exit code.
		appLog.ErrorContext(ctx, "connecting to database failed", slog.Any("error", err))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	orderRepo := orderpg.New(pool)
	paymentRepo := paymentpg.New(pool)
	inventoryRepo := inventorypg.New(pool)
	promotionRepo := promotionpg.New(pool)
	notificationRepo := notificationpg.New(pool)

	txRunner := database.NewTxRunner(pool)

	inventorySvc := inventory.NewService(inventoryRepo)
	promotionSvc := promotion.NewService(promotionRepo, txRunner)
	notificationSvc := notification.NewService(notificationRepo, appLog)

	orderSvc := bootstrap.NewOrderService(orderRepo, txRunner, nil, inventorySvc, promotionSvc, nil, appLog)

	gw := mockgateway.New(cfg.Payment.GatewayURL, cfg.Payment.GatewayTimeout)

	paymentSvc := bootstrap.NewPaymentService(paymentRepo, txRunner, gw, orderSvc, inventorySvc, promotionSvc, appLog)

	jobCfg := jobs.Config{
		Interval:      cfg.Worker.Interval,
		BatchSize:     cfg.Worker.BatchSize,
		LeaseDuration: cfg.Worker.LeaseDuration,
		Concurrency:   cfg.Worker.Concurrency,
		PruneAge:      cfg.Worker.PruneAge,
		PruneLimit:    cfg.Worker.PruneLimit,
	}

	paymentProcessor := paymentworker.NewProcessor(paymentSvc, bootstrap.NewOrderHousekeeper(orderSvc), appLog)
	paymentRunner := jobs.NewRunner("payment", paymentRepo, paymentProcessor, jobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", notificationRepo, notificationSvc, jobCfg, appLog)

	appLog.InfoContext(ctx, "worker starting", slog.String("env", cfg.App.Env))
	var wg sync.WaitGroup
	for _, start := range []func(context.Context){paymentRunner.Start, notificationRunner.Start} {
		wg.Go(func() {
			start(ctx)
		})
	}
	wg.Wait()
	appLog.InfoContext(ctx, "worker stopped")
	return nil
}
