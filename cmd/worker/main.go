package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
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
	infra, err := config.Load()
	if err != nil {
		// No logger yet by construction: the log settings live in the config that
		// just failed. Report to stderr and let the caller own the exit code.
		fmt.Fprintf(os.Stderr, "loading infra config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(infra.Log.Level, infra.Log.Format)

	// Infra must load first: payment and order both validate their own timeouts
	// against infra.Worker.LeaseDuration.
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

	// order.Config itself has no reader here -- RateLimit/RateWindow are for the
	// API's HTTP limiter -- but the worker is the process that actually leases
	// jobs, so it is the right place to also catch a WORKER_LEASE_DURATION that
	// outlives order's stale-processing threshold.
	if _, err = order.LoadConfig(infra.Worker.LeaseDuration); err != nil {
		appLog.Error("loading order config failed", slog.String("error", err.Error()))
		return err
	}

	paymentCfg, err := payment.LoadConfig(infra.App.Env, infra.Worker.LeaseDuration)
	if err != nil {
		appLog.Error("loading payment config failed", slog.String("error", err.Error()))
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, infra.Database)
	if err != nil {
		// Reported as well as returned: main only sets the exit code.
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	app, err := bootstrap.New(bootstrap.Deps{
		Auth:    authCfg,
		Cart:    cartCfg,
		Payment: paymentCfg,
		Pool:    pool,
		Logger:  appLog,
	})
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	jobCfg := jobs.Config{
		Interval:      infra.Worker.Interval,
		BatchSize:     infra.Worker.BatchSize,
		LeaseDuration: infra.Worker.LeaseDuration,
		Concurrency:   infra.Worker.Concurrency,
		PruneAge:      infra.Worker.PruneAge,
		PruneLimit:    infra.Worker.PruneLimit,
	}

	// payment.Service satisfies jobs.Processor directly: a service holds no pool
	// (rule 9), and the queue side still needs the bare repository. notification's
	// Jobs is both at once, so it needs no such split.
	paymentRunner := jobs.NewRunner("payment", paymentpg.New(pool), app.Payments, jobCfg, appLog)
	notificationRunner := jobs.NewRunner("notification", app.Notifications.Jobs, app.Notifications.Jobs, jobCfg, appLog)

	appLog.InfoContext(ctx, "worker starting", slog.String("env", infra.App.Env))
	var wg sync.WaitGroup
	for _, start := range []func(context.Context){paymentRunner.Start, notificationRunner.Start} {
		wg.Go(func() {
			start(ctx)
		})
	}
	// Expire and RecoverStale are order's own housekeeping sweeps, not job
	// queues -- there is nothing to claim or lease -- so they run on a plain
	// ticker instead of a jobs.Runner, independent of payment's queue tick.
	wg.Go(func() { runSweep(ctx, "order-expire", infra.Worker.Interval, app.Orders.Expire.Sweep, appLog) })
	wg.Go(func() {
		runSweep(ctx, "order-recoverstale", infra.Worker.Interval, app.Orders.RecoverStale.Sweep, appLog)
	})
	wg.Wait()
	appLog.InfoContext(ctx, "worker stopped")
	return nil
}

// runSweep drives a housekeeping sweep with no queue behind it. Recovered
// separately per tick, same as jobs.Runner.tick, so a panic here takes down
// only this sweep's next tick, never the whole worker binary.
func runSweep(
	ctx context.Context,
	name string,
	interval time.Duration,
	sweep func(context.Context) error,
	log *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepOnce(ctx, name, sweep, log)
		}
	}
}

func sweepOnce(ctx context.Context, name string, sweep func(context.Context) error, log *slog.Logger) {
	defer func() {
		if rec := recover(); rec != nil {
			log.ErrorContext(ctx, name+" sweep panicked", slog.String("panic", fmt.Sprint(rec)))
		}
	}()
	if err := sweep(ctx); err != nil {
		log.ErrorContext(ctx, name+" sweep failed", slog.String("error", err.Error()))
	}
}
