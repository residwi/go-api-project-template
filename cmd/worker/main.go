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
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	paymentworker "github.com/residwi/go-api-project-template/internal/modules/payment/worker"
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
	infra, err := config.LoadInfra()
	if err != nil {
		// No logger yet by construction: the log settings live in the config that
		// just failed. Report to stderr and let the caller own the exit code.
		fmt.Fprintf(os.Stderr, "loading infra config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(infra.Log.Level, infra.Log.Format)

	cfg, err := config.Load()
	if err != nil {
		appLog.Error("loading config failed", slog.String("error", err.Error()))
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		// Reported as well as returned: main only sets the exit code.
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	app, err := bootstrap.New(bootstrap.Deps{
		Infra:  infra,
		Config: cfg,
		Pool:   pool,
		Logger: appLog,
	})
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	jobCfg := jobs.Config{
		Interval:      cfg.Worker.Interval,
		BatchSize:     cfg.Worker.BatchSize,
		LeaseDuration: cfg.Worker.LeaseDuration,
		Concurrency:   cfg.Worker.Concurrency,
		PruneAge:      cfg.Worker.PruneAge,
		PruneLimit:    cfg.Worker.PruneLimit,
	}

	// app.Orders satisfies payment.OrderHousekeeper directly, so the processor
	// needs no adapter. The queue side still needs the bare repository: a
	// service holds no pool (rule 9), but Runner drains payment_jobs/
	// notifications by Claim/Prune on the repository itself, not through the
	// service.
	paymentProcessor := paymentworker.NewProcessor(app.Payments, app.Orders, appLog)
	paymentRunner := jobs.NewRunner("payment", paymentpg.New(pool), paymentProcessor, jobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", notificationpg.New(pool), app.Notifications, jobCfg, appLog)

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
