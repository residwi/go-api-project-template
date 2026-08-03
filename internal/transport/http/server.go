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
// trigger. Tests use this instead of Run to stop the server without signalling
// the whole process.
func RunContext(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Setup(cfg.Log.Level, cfg.Log.Format)

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	readerPool, err := database.NewReaderPostgres(ctx, cfg.Database)
	if err != nil {
		if !errors.Is(err, apperror.ErrReaderNotConfigured) {
			slog.WarnContext(ctx, "failed to connect reader database, using primary", "error", err)
		}
		readerPool = nil
	}
	if readerPool != nil {
		defer readerPool.Close()
	}

	rdb, err := cache.NewRedis(ctx, cfg.Redis)
	if err != nil {
		slog.WarnContext(ctx, "failed to connect to redis, continuing without cache/rate-limiting", "error", err)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	deps := &Deps{
		Config:     cfg,
		Pool:       pool,
		ReaderPool: readerPool,
		Cache:      rdb,
	}

	router := NewRouter(deps)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      router.Handler,
		ReadTimeout:  cfg.App.ReadTimeout,
		WriteTimeout: cfg.App.WriteTimeout,
		IdleTimeout:  cfg.App.IdleTimeout,
	}

	go func() {
		slog.InfoContext(ctx, "server starting", "port", cfg.App.Port, "env", cfg.App.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.InfoContext(ctx, "server stopped gracefully")
	return nil
}

type Deps struct {
	Config     *config.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	// Cache is the shared Redis connection, named for its principal consumer.
	Cache *redis.Client
}
