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

	// A charge is retried up to three times inside one job, so the whole attempt
	// must finish before order sweeps the order as stale -- otherwise a retry
	// races a sweep that has already released the stock. Neither module owns
	// this rule, so it is checked where both values are in scope.
	if paymentCfg.GatewayTimeout*3 >= order.StaleProcessingThreshold {
		return cfg, fmt.Errorf(
			"PAYMENT_GATEWAY_TIMEOUT (%s) is too large: 3x it must stay below the order "+
				"stale-processing threshold (%s) so a valid PAYMENT_JOB_TIMEOUT range exists",
			paymentCfg.GatewayTimeout, order.StaleProcessingThreshold,
		)
	}

	return Config{
		Auth:         authCfg,
		Cart:         cartCfg,
		Notification: notificationCfg,
		Order:        orderCfg,
		Payment:      paymentCfg,
	}, nil
}
