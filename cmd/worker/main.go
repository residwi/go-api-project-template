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
	paymentdomain "github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

// paymentProcessor routes a claimed payment job to app.Payments.JobProcessor
// and, once per tick, runs order's stale-order housekeeping. It lives here
// rather than in payment because the sweep is order's, and payment has no
// business declaring a port for it -- payment/worker used to exist solely
// to bolt this cross-module composition onto the dispatcher, which is
// exactly the composition root's job.
type paymentProcessor struct {
	dispatcher jobs.LegacyProcessor[paymentdomain.Job]
	recover    func(context.Context) error
	expire     func(context.Context) error
	logger     *slog.Logger
}

func (p paymentProcessor) Process(ctx context.Context, job paymentdomain.Job) error {
	return p.dispatcher.Process(ctx, job)
}

// Sweep mirrors the deleted payment/worker.Processor exactly: recovery runs
// first, and a recovery failure is logged and swallowed so expiry still
// runs every tick; an expiry failure is returned untouched (not logged
// here) so the runner's own generic "sweep failed" log captures it, same as
// it did before this move.
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

	jobCfg := jobs.Config{
		Interval:      infra.Worker.Interval,
		BatchSize:     infra.Worker.BatchSize,
		LeaseDuration: infra.Worker.LeaseDuration,
		Concurrency:   infra.Worker.Concurrency,
		PruneAge:      infra.Worker.PruneAge,
		PruneLimit:    infra.Worker.PruneLimit,
	}

	proc := paymentProcessor{
		dispatcher: app.Payments.JobProcessor,
		recover:    app.Orders.RecoverStale,
		expire:     app.Orders.ExpireStale,
		logger:     appLog,
	}
	// app.Payments satisfies jobs.LegacyQueue[paymentdomain.Job] directly, via
	// Claim/Prune promoted straight onto the Service (see payment/jobs.go),
	// now that the queue lives in payment's own root package.
	paymentRunner := jobs.NewLegacyRunner("payment", app.Payments, proc, jobCfg, appLog)
	notificationRunner := jobs.NewLegacyRunner(
		"notification",
		app.Notifications.Jobs,
		app.Notifications.Jobs,
		jobCfg,
		appLog,
	)

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
