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
	"github.com/residwi/go-api-project-template/internal/platform/config"
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
	appCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading app config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(appCfg.Log.Level, appCfg.Log.Format)

	modCfg, err := bootstrap.LoadConfig(appCfg)
	if err != nil {
		appLog.Error("loading module config failed", slog.String("error", err.Error()))
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

	app, err := bootstrap.New(modCfg, database.DB{Primary: primaryDB}, nil, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	paymentJobCfg := jobs.Config{
		Interval:      modCfg.Payment.JobInterval,
		BatchSize:     appCfg.Worker.BatchSize,
		LeaseDuration: modCfg.Payment.JobTimeout,
		Concurrency:   modCfg.Payment.JobConcurrency,
		PruneAge:      appCfg.Worker.PruneAge,
		PruneLimit:    appCfg.Worker.PruneLimit,
	}

	notificationJobCfg := jobs.Config{
		Interval:      modCfg.Notification.JobInterval,
		BatchSize:     appCfg.Worker.BatchSize,
		LeaseDuration: modCfg.Notification.JobTimeout,
		Concurrency:   modCfg.Notification.JobConcurrency,
		PruneAge:      appCfg.Worker.PruneAge,
		PruneLimit:    appCfg.Worker.PruneLimit,
	}

	orderJobCfg := jobs.Config{
		Interval:      modCfg.Order.JobInterval,
		BatchSize:     appCfg.Worker.BatchSize,
		LeaseDuration: modCfg.Order.JobLease,
		Concurrency:   modCfg.Order.JobConcurrency,
		PruneAge:      appCfg.Worker.PruneAge,
		PruneLimit:    appCfg.Worker.PruneLimit,
	}

	if err := app.Orders.ScheduleExpireStale(ctx, orderJobCfg.Interval); err != nil {
		appLog.ErrorContext(ctx, "scheduling order expiry sweep failed", slog.String("error", err.Error()))
		return fmt.Errorf("scheduling order expiry sweep: %w", err)
	}

	paymentRunner := jobs.NewRunner("payment", app.JobStore, app.Jobs, paymentJobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", app.JobStore, app.Jobs, notificationJobCfg, appLog)
	orderRunner := jobs.NewRunner("order", app.JobStore, app.Jobs, orderJobCfg, appLog)

	appLog.InfoContext(ctx, "worker starting", slog.String("env", appCfg.App.Env))
	var wg sync.WaitGroup
	for _, start := range []func(context.Context){paymentRunner.Start, notificationRunner.Start, orderRunner.Start} {
		wg.Go(func() {
			start(ctx)
		})
	}
	wg.Wait()
	appLog.InfoContext(ctx, "worker stopped")
	return nil
}
