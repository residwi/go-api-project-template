package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
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

	if _, err := bootstrap.New(modCfg, database.DB{Primary: primaryDB}, nil, appLog); err != nil {
		appLog.ErrorContext(ctx, "wiring services failed", slog.String("error", err.Error()))
		return fmt.Errorf("wiring services: %w", err)
	}

	appLog.InfoContext(ctx, "worker starting", slog.String("env", appCfg.App.Env))
	<-ctx.Done()
	appLog.InfoContext(ctx, "worker stopped")
	return nil
}
