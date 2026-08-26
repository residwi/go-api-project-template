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
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

type paymentProcessor struct {
	registry jobs.Processor
	recover  func(context.Context) error
	expire   func(context.Context) error
	logger   *slog.Logger
}

func (p paymentProcessor) Process(ctx context.Context, rec jobs.Record) error {
	return p.registry.Process(ctx, rec)
}

func (p paymentProcessor) Sweep(ctx context.Context) error {
	if err := p.recover(ctx); err != nil {
		p.logger.ErrorContext(ctx, "recover stale processing orders failed", slog.String("error", err.Error()))
	}
	return p.expire(ctx)
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	appCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading app config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(appCfg.Log.Level, appCfg.Log.Format)

	authCfg, err := auth.LoadConfig()
	if err != nil {
		appLog.Error("loading auth config failed", slog.String("error", err.Error()))
		return err
	}

	cartCfg, err := cart.LoadConfig()
	if err != nil {
		appLog.Error("loading cart config failed", slog.String("error", err.Error()))
		return err
	}

	if _, err = order.LoadConfig(appCfg.Worker.PaymentLeaseDuration); err != nil {
		appLog.Error("loading order config failed", slog.String("error", err.Error()))
		return err
	}

	paymentCfg, err := payment.LoadConfig(appCfg.App.Env, appCfg.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.Error("loading payment config failed", slog.String("error", err.Error()))
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	primaryDB, err := database.NewPrimaryPostgres(ctx, appCfg.Database)
	if err != nil {
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer primaryDB.Close()

	app, err := bootstrap.New(
		authCfg,
		cartCfg,
		paymentCfg,
		database.DB{Primary: primaryDB},
		nil,
		appLog,
	)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	paymentJobCfg := jobs.Config{
		Interval:      appCfg.Worker.PaymentInterval,
		BatchSize:     appCfg.Worker.BatchSize,
		LeaseDuration: appCfg.Worker.PaymentLeaseDuration,
		Concurrency:   appCfg.Worker.PaymentConcurrency,
		PruneAge:      appCfg.Worker.PruneAge,
		PruneLimit:    appCfg.Worker.PruneLimit,
	}

	notificationJobCfg := jobs.Config{
		Interval:      appCfg.Worker.NotificationInterval,
		BatchSize:     appCfg.Worker.BatchSize,
		LeaseDuration: appCfg.Worker.NotificationLease,
		Concurrency:   appCfg.Worker.NotificationConcurrency,
		PruneAge:      appCfg.Worker.PruneAge,
		PruneLimit:    appCfg.Worker.PruneLimit,
	}

	proc := paymentProcessor{
		registry: app.Jobs,
		recover:  app.Orders.RecoverStale,
		expire:   app.Orders.ExpireStale,
		logger:   appLog,
	}

	paymentRunner := jobs.NewRunner("payment", app.JobStore, proc, paymentJobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", app.JobStore, app.Jobs, notificationJobCfg, appLog)

	appLog.InfoContext(ctx, "worker starting", slog.String("env", appCfg.App.Env))
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
