package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/residwi/go-api-project-template/internal/app"
	"github.com/residwi/go-api-project-template/internal/config"
	notificationjobs "github.com/residwi/go-api-project-template/internal/features/notification/adapter/jobs"
	"github.com/residwi/go-api-project-template/internal/features/order"
	orderjobs "github.com/residwi/go-api-project-template/internal/features/order/adapter/jobs"
	paymentjobs "github.com/residwi/go-api-project-template/internal/features/payment/adapter/jobs"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

const softStopDivisor = 2

func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return RunContext(ctx)
}

func RunContext(ctx context.Context) error {
	appCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading app config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(appCfg.Log.Level, appCfg.Log.Format)

	modCfg, err := app.LoadConfig(appCfg)
	if err != nil {
		appLog.ErrorContext(ctx, "loading module config failed", slog.String("error", err.Error()))
		return err
	}

	primaryDB, err := database.NewPrimaryPostgres(ctx, app.PoolOptions(appCfg.Database))
	if err != nil {
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer primaryDB.Close()

	db := database.DB{Primary: primaryDB}

	deps, err := app.New(modCfg, db, nil, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	softStop := appCfg.App.ShutdownTimeout / softStopDivisor

	client, err := newClient(db, modCfg, deps, appCfg.Worker.RescueAfter, softStop, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "building job client failed", slog.String("error", err.Error()))
		return err
	}

	if err := client.Start(ctx); err != nil {
		appLog.ErrorContext(ctx, "starting job client failed", slog.String("error", err.Error()))
		return fmt.Errorf("starting job client: %w", err)
	}

	appLog.InfoContext(ctx, "worker starting", slog.String("env", appCfg.App.Env))
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), appCfg.App.ShutdownTimeout)
	defer cancel()

	if err := client.Stop(shutdownCtx); err != nil {
		appLog.ErrorContext(ctx, "worker shutdown failed", slog.String("error", err.Error()))
		return fmt.Errorf("worker shutdown: %w", err)
	}

	appLog.InfoContext(ctx, "worker stopped")
	return nil
}

func newClient(
	db database.DB,
	modCfg app.Config,
	deps *app.Services,
	rescueAfter time.Duration,
	softStop time.Duration,
	appLog *slog.Logger,
) (*river.Client[pgx.Tx], error) {
	if rescueAfter <= 0 || rescueAfter >= order.StaleProcessingThreshold {
		return nil, errors.New(
			"WORKER_RESCUE_AFTER must be less than the order stale-processing threshold, " +
				"or the sweep can revert an order whose charge is still running",
		)
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, paymentjobs.NewRefundWorker(deps.Payments, modCfg.Payment.JobTimeout))
	river.AddWorker(workers, notificationjobs.NewSendWorker(deps.Notifications, modCfg.Notification.JobTimeout))
	river.AddWorker(workers, orderjobs.NewExpireStaleWorker(deps.Orders, appLog, modCfg.Order.JobTimeout))

	return river.NewClient(riverpgxv5.New(db.Primary), &river.Config{
		Workers:              workers,
		RescueStuckJobsAfter: rescueAfter,
		SoftStopTimeout:      softStop,
		Logger:               appLog,
		Queues: map[string]river.QueueConfig{
			"payment": {
				MaxWorkers:        modCfg.Payment.JobConcurrency,
				FetchPollInterval: modCfg.Payment.JobInterval,
			},
			"notification": {
				MaxWorkers:        modCfg.Notification.JobConcurrency,
				FetchPollInterval: modCfg.Notification.JobInterval,
			},
			"order": {MaxWorkers: modCfg.Order.JobConcurrency},
		},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(modCfg.Order.JobInterval),
				func() (river.JobArgs, *river.InsertOpts) { return orderjobs.ExpireStaleArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	})
}
