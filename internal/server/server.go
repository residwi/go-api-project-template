package server

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

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Setup(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	readerPool, err := database.NewReaderPostgres(ctx, cfg.Database.ReaderURL)
	if err != nil {
		if !errors.Is(err, core.ErrReaderNotConfigured) {
			slog.Warn("failed to connect reader database, using primary", "error", err)
		}
		readerPool = nil
	}
	if readerPool != nil {
		defer readerPool.Close()
	}

	rdb, err := cache.NewRedis(ctx, cfg.Redis)
	if err != nil {
		slog.Warn("failed to connect to redis, continuing without cache/rate-limiting", "error", err)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	deps := &Deps{
		Config:     cfg,
		Pool:       pool,
		ReaderPool: readerPool,
		Redis:      rdb,
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
		slog.Info("server starting", "port", cfg.App.Port, "env", cfg.App.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped gracefully")
	return nil
}

type Deps struct {
	Config     *config.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	Redis      *redis.Client
}
