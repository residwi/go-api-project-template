package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

// Run serves until the process receives SIGINT or SIGTERM.
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return RunContext(ctx)
}

// RunContext serves until ctx is cancelled, so the caller owns the shutdown
// trigger -- which is how tests stop it without signalling the whole process.
func RunContext(ctx context.Context) error {
	infra, err := config.Load()
	if err != nil {
		// No logger yet by construction: the log settings live in the config that
		// just failed. Report to stderr and let the caller own the exit code.
		fmt.Fprintf(os.Stderr, "loading infra config failed: %v\n", err)
		return err
	}

	appLog := logger.Setup(infra.Log.Level, infra.Log.Format)

	authCfg, cartCfg, orderCfg, paymentCfg, err := loadModuleConfigs(ctx, infra, appLog)
	if err != nil {
		return err
	}

	pool, err := database.NewPostgres(ctx, infra.Database)
	if err != nil {
		// Reported as well as returned: main only sets the exit code.
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	readerPool, err := database.NewReaderPostgres(ctx, infra.Database)
	if err != nil {
		if !errors.Is(err, apperror.ErrReaderNotConfigured) {
			appLog.WarnContext(
				ctx,
				"failed to connect reader database, using primary",
				slog.String("error", err.Error()),
			)
		}
		readerPool = nil
	}
	if readerPool != nil {
		defer readerPool.Close()
	}

	rdb, err := cache.NewRedis(ctx, infra.Redis)
	if err != nil {
		appLog.WarnContext(
			ctx,
			"failed to connect to redis, continuing without cache/rate-limiting",
			slog.String("error", err.Error()),
		)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	deps := &Deps{
		Infra:      infra,
		Auth:       authCfg,
		Order:      orderCfg,
		Payment:    paymentCfg,
		Pool:       pool,
		ReaderPool: readerPool,
		Cache:      rdb,
		Logger:     appLog,
	}

	app, err := bootstrap.New(bootstrap.Deps{
		Auth:       authCfg,
		Cart:       cartCfg,
		Payment:    paymentCfg,
		Pool:       pool,
		ReaderPool: readerPool,
		Cache:      rdb,
		Logger:     appLog,
	})
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	handler := NewRouter(deps, app)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", infra.App.Port),
		Handler:      handler,
		ReadTimeout:  infra.App.ReadTimeout,
		WriteTimeout: infra.App.WriteTimeout,
		IdleTimeout:  infra.App.IdleTimeout,
		// net/http reports its own problems here -- superfluous WriteHeader calls, TLS
		// handshake failures -- which would otherwise go to plain-text stderr.
		ErrorLog: slog.NewLogLogger(appLog.Handler(), slog.LevelError),
	}

	// Buffered so the send never blocks: on clean shutdown the select below has
	// already taken ctx.Done and nobody receives.
	serveErr := make(chan error, 1)
	go func() {
		appLog.InfoContext(ctx, "server starting", slog.Int("port", infra.App.Port), slog.String("env", infra.App.Env))
		serveErr <- srv.ListenAndServe()
	}()

	// A bind failure must abort: blocking on ctx.Done alone left the binary alive,
	// serving nothing, exiting 0 -- which reads as a healthy rollout.
	select {
	case err := <-serveErr:
		// ErrServerClosed is the Shutdown below, not a failure.
		if !errors.Is(err, http.ErrServerClosed) {
			appLog.ErrorContext(ctx, "server failed to start", slog.String("error", err.Error()))
			return fmt.Errorf("starting server: %w", err)
		}
	case <-ctx.Done():
	}

	appLog.InfoContext(ctx, "shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), infra.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLog.ErrorContext(ctx, "server shutdown failed", slog.String("error", err.Error()))
		return fmt.Errorf("server shutdown: %w", err)
	}

	appLog.InfoContext(ctx, "server stopped gracefully")
	return nil
}

// loadModuleConfigs loads every module config RunContext needs, in the one
// order that is safe: infra first, since payment and order each validate
// their own timeout against infra.Worker.LeaseDuration. Each LoadConfig
// already names itself in the errors it returns, so this does not wrap them
// again -- only the "which one failed" log line is added here.
func loadModuleConfigs(
	ctx context.Context,
	infra *config.Settings,
	appLog *slog.Logger,
) (auth.Config, cart.Config, order.Config, payment.Config, error) {
	var cartCfg cart.Config
	var orderCfg order.Config
	var paymentCfg payment.Config

	authCfg, err := auth.LoadConfig()
	if err != nil {
		appLog.ErrorContext(ctx, "loading auth config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	cartCfg, err = cart.LoadConfig()
	if err != nil {
		appLog.ErrorContext(ctx, "loading cart config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	orderCfg, err = order.LoadConfig(infra.Worker.LeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading order config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	paymentCfg, err = payment.LoadConfig(infra.App.Env, infra.Worker.LeaseDuration)
	if err != nil {
		appLog.ErrorContext(ctx, "loading payment config failed", slog.String("error", err.Error()))
		return authCfg, cartCfg, orderCfg, paymentCfg, err
	}

	return authCfg, cartCfg, orderCfg, paymentCfg, nil
}

// Deps is what NewRouter needs beyond the wired App: infra's CORS/App.Env
// (genuinely cross-cutting, not any one module's), plus the module configs
// its middleware reads (auth and order's rate limits, payment's webhook
// secret). Cart's config has no reader here -- MaxItems only matters to
// bootstrap.New, not to routing.
type Deps struct {
	Infra      *config.Settings
	Auth       auth.Config
	Order      order.Config
	Payment    payment.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	Cache      *redis.Client
	Logger     *slog.Logger
}
