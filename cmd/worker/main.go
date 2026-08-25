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
	infra, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading infra config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(infra.Log.Level, infra.Log.Format)

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

	if _, err = order.LoadConfig(infra.Worker.PaymentLeaseDuration); err != nil {
		appLog.Error("loading order config failed", slog.String("error", err.Error()))
		return err
	}

	paymentCfg, err := payment.LoadConfig(infra.App.Env, infra.Worker.PaymentLeaseDuration)
	if err != nil {
		appLog.Error("loading payment config failed", slog.String("error", err.Error()))
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, infra.Database)
	if err != nil {
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	app, err := bootstrap.New(
		authCfg,
		cartCfg,
		paymentCfg,
		database.DB{Primary: pool},
		nil,
		appLog,
	)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	paymentJobCfg := jobs.Config{
		Interval:      infra.Worker.PaymentInterval,
		BatchSize:     infra.Worker.BatchSize,
		LeaseDuration: infra.Worker.PaymentLeaseDuration,
		Concurrency:   infra.Worker.PaymentConcurrency,
		PruneAge:      infra.Worker.PruneAge,
		PruneLimit:    infra.Worker.PruneLimit,
	}

	notificationJobCfg := jobs.Config{
		Interval:      infra.Worker.NotificationInterval,
		BatchSize:     infra.Worker.BatchSize,
		LeaseDuration: infra.Worker.NotificationLease,
		Concurrency:   infra.Worker.NotificationConcurrency,
		PruneAge:      infra.Worker.PruneAge,
		PruneLimit:    infra.Worker.PruneLimit,
	}

	proc := paymentProcessor{
		registry: app.Jobs,
		recover:  app.Orders.RecoverStale,
		expire:   app.Orders.ExpireStale,
		logger:   appLog,
	}

	paymentRunner := jobs.NewRunner("payment", app.JobStore, proc, paymentJobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", app.JobStore, app.Jobs, notificationJobCfg, appLog)

	appLog.InfoContext(ctx, "worker starting", slog.String("env", infra.App.Env))
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
