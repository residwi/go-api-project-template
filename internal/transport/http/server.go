package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
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
	cfg, err := config.Load()
	if err != nil {
		logger.FromEnv().ErrorContext(ctx, "loading config failed", slog.String("error", err.Error()))
		return fmt.Errorf("loading config: %w", err)
	}

	appLog := logger.Setup(cfg.Log.Level, cfg.Log.Format)

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		// Reported as well as returned: main only sets the exit code.
		appLog.ErrorContext(ctx, "connecting to database failed", slog.String("error", err.Error()))
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	readerPool, err := database.NewReaderPostgres(ctx, cfg.Database)
	if err != nil {
		if !errors.Is(err, apperror.ErrReaderNotConfigured) {
			appLog.WarnContext(ctx, "failed to connect reader database, using primary", slog.Any("error", err))
		}
		readerPool = nil
	}
	if readerPool != nil {
		defer readerPool.Close()
	}

	rdb, err := cache.NewRedis(ctx, cfg.Redis)
	if err != nil {
		appLog.WarnContext(
			ctx,
			"failed to connect to redis, continuing without cache/rate-limiting",
			slog.Any("error", err),
		)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	deps := &Deps{
		Config:     cfg,
		Pool:       pool,
		ReaderPool: readerPool,
		Cache:      rdb,
		Logger:     appLog,
	}

	handler := NewRouter(deps)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      handler,
		ReadTimeout:  cfg.App.ReadTimeout,
		WriteTimeout: cfg.App.WriteTimeout,
		IdleTimeout:  cfg.App.IdleTimeout,
		// net/http reports its own problems here -- superfluous WriteHeader calls, TLS
		// handshake failures -- which would otherwise go to plain-text stderr.
		ErrorLog: slog.NewLogLogger(appLog.Handler(), slog.LevelError),
	}

	// Buffered so the send never blocks: on clean shutdown the select below has
	// already taken ctx.Done and nobody receives.
	serveErr := make(chan error, 1)
	go func() {
		appLog.InfoContext(ctx, "server starting", slog.Int("port", cfg.App.Port), slog.String("env", cfg.App.Env))
		serveErr <- srv.ListenAndServe()
	}()

	// A bind failure must abort: blocking on ctx.Done alone left the binary alive,
	// serving nothing, exiting 0 -- which reads as a healthy rollout.
	select {
	case err := <-serveErr:
		// ErrServerClosed is the Shutdown below, not a failure.
		if !errors.Is(err, http.ErrServerClosed) {
			appLog.ErrorContext(ctx, "server failed to start", slog.Any("error", err))
			return fmt.Errorf("starting server: %w", err)
		}
	case <-ctx.Done():
	}

	appLog.InfoContext(ctx, "shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLog.ErrorContext(ctx, "server shutdown failed", slog.Any("error", err))
		return fmt.Errorf("server shutdown: %w", err)
	}

	appLog.InfoContext(ctx, "server stopped gracefully")
	return nil
}

type Deps struct {
	Config     *config.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	Cache      *redis.Client
	Logger     *slog.Logger
}
