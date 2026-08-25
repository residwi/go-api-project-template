package payment

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"

	"github.com/residwi/go-api-project-template/internal/modules/order"
)

type Config struct {
	Gateway        string        `envconfig:"PAYMENT_GATEWAY"         default:"mock"`
	GatewayURL     string        `envconfig:"PAYMENT_GATEWAY_URL"     default:"http://localhost:8080/mock/payment"`
	GatewayTimeout time.Duration `envconfig:"PAYMENT_GATEWAY_TIMEOUT" default:"10s"`
	GatewayAPIKey  string        `envconfig:"PAYMENT_GATEWAY_API_KEY" default:""`
	WebhookSecret  string        `envconfig:"PAYMENT_WEBHOOK_SECRET"  default:"webhook-secret"`
}

const defaultWebhookSecret = "webhook-secret"

const (
	gatewayMock     = "mock"
	gatewayStripe   = "stripe"
	gatewayMidtrans = "midtrans"
)

func LoadConfig(appEnv string, jobsLease time.Duration) (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading payment config: %w", err)
	}

	if cfg.Gateway != gatewayMock && cfg.Gateway != gatewayStripe && cfg.Gateway != gatewayMidtrans {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY must be %q, %q or %q, got %q", gatewayMock, gatewayStripe, gatewayMidtrans, cfg.Gateway,
		)
	}

	if appEnv != "development" && cfg.WebhookSecret == defaultWebhookSecret {
		return Config{}, errors.New("PAYMENT_WEBHOOK_SECRET must be set in non-development environments")
	}

	if jobsLease < cfg.GatewayTimeout*3 {
		return Config{}, errors.New(
			"WORKER_PAYMENT_LEASE must be at least 3× PAYMENT_GATEWAY_TIMEOUT to avoid duplicate gateway calls",
		)
	}

	if cfg.GatewayTimeout*3 >= order.StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY_TIMEOUT (%s) is too large: 3× it must stay below the order stale-processing threshold (%s) so a valid WORKER_PAYMENT_LEASE range exists",
			cfg.GatewayTimeout,
			order.StaleProcessingThreshold,
		)
	}

	return cfg, nil
}
