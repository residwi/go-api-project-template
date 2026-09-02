package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/app"
	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

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

	replicaDB, err := database.NewReplicaPostgres(ctx, app.ReplicaPoolOptions(appCfg.Database))
	if err != nil {
		if !errors.Is(err, database.ErrReplicaNotConfigured) {
			appLog.WarnContext(
				ctx,
				"failed to connect replica database, using primary",
				slog.String("error", err.Error()),
			)
		}
		replicaDB = nil
	}
	if replicaDB != nil {
		defer replicaDB.Close()
	}

	rdb, err := cache.NewRedis(ctx, &redis.Options{
		Addr:         appCfg.Redis.Addr(),
		Password:     appCfg.Redis.Password,
		DB:           appCfg.Redis.DB,
		PoolSize:     appCfg.Redis.PoolSize,
		MinIdleConns: appCfg.Redis.MinIdleConns,
		DialTimeout:  appCfg.Redis.DialTimeout,
		ReadTimeout:  appCfg.Redis.ReadTimeout,
		WriteTimeout: appCfg.Redis.WriteTimeout,
		PoolTimeout:  appCfg.Redis.PoolTimeout,
	})
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

	db := database.DB{Primary: primaryDB, Replica: replicaDB}

	deps, err := app.New(modCfg, db, rdb, appLog)
	if err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	handler := NewRouter(appCfg, modCfg, rdb, appLog, deps)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", appCfg.App.Port),
		Handler:      handler,
		ReadTimeout:  appCfg.App.ReadTimeout,
		WriteTimeout: appCfg.App.WriteTimeout,
		IdleTimeout:  appCfg.App.IdleTimeout,
		ErrorLog:     slog.NewLogLogger(appLog.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		appLog.InfoContext(
			ctx,
			"server starting",
			slog.Int("port", appCfg.App.Port),
			slog.String("env", appCfg.App.Env),
		)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			appLog.ErrorContext(ctx, "server failed to start", slog.String("error", err.Error()))
			return fmt.Errorf("starting server: %w", err)
		}
	case <-ctx.Done():
	}

	appLog.InfoContext(ctx, "shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), appCfg.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLog.ErrorContext(ctx, "server shutdown failed", slog.String("error", err.Error()))
		return fmt.Errorf("server shutdown: %w", err)
	}

	appLog.InfoContext(ctx, "server stopped gracefully")
	return nil
}
