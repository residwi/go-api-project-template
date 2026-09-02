package app

import (
	"fmt"

	"github.com/residwi/go-api-project-template/internal/config"
	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Config struct {
	Auth         auth.Config
	Cart         cart.Config
	Notification notification.Config
	Order        order.Config
	Payment      payment.Config
}

func PoolOptions(cfg config.Database) database.PostgresOptions {
	return database.PostgresOptions{
		DSN:             cfg.DSN(),
		MaxConns:        cfg.MaxConns,
		MinConns:        cfg.MinConns,
		MaxConnLifetime: cfg.MaxConnLifetime,
		MaxConnIdleTime: cfg.MaxConnIdleTime,
	}
}

func ReplicaPoolOptions(cfg config.Database) database.PostgresOptions {
	opts := PoolOptions(cfg)
	opts.DSN = cfg.ReplicaURL

	return opts
}

func LoadConfig(appCfg *config.Settings) (Config, error) {
	var cfg Config

	authCfg, err := auth.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading auth config: %w", err)
	}

	cartCfg, err := cart.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading cart config: %w", err)
	}

	notificationCfg, err := notification.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading notification config: %w", err)
	}

	orderCfg, err := order.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading order config: %w", err)
	}

	paymentCfg, err := payment.LoadConfig(appCfg.App.Env)
	if err != nil {
		return cfg, fmt.Errorf("loading payment config: %w", err)
	}

	return Config{
		Auth:         authCfg,
		Cart:         cartCfg,
		Notification: notificationCfg,
		Order:        orderCfg,
		Payment:      paymentCfg,
	}, nil
}
