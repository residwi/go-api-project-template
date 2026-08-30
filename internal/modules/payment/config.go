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

	JobInterval    time.Duration `envconfig:"PAYMENT_JOB_INTERVAL"    default:"10s"`
	JobConcurrency int           `envconfig:"PAYMENT_JOB_CONCURRENCY" default:"5"`
	JobTimeout     time.Duration `envconfig:"PAYMENT_JOB_TIMEOUT"     default:"2m"`
}

const defaultWebhookSecret = "webhook-secret"

const minJobInterval = 5 * time.Second

const (
	gatewayMock     = "mock"
	gatewayStripe   = "stripe"
	gatewayMidtrans = "midtrans"
)

func LoadConfig(appEnv string) (Config, error) {
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

	if cfg.JobInterval < minJobInterval {
		return Config{}, errors.New("PAYMENT_JOB_INTERVAL must be at least 5s to avoid database polling overhead")
	}

	if cfg.JobConcurrency < 1 {
		return Config{}, errors.New(
			"PAYMENT_JOB_CONCURRENCY must be at least 1 (River requires QueueConfig.MaxWorkers to be at least 1)",
		)
	}

	if cfg.JobTimeout < cfg.GatewayTimeout*3 {
		return Config{}, errors.New(
			"PAYMENT_JOB_TIMEOUT must be at least 3× PAYMENT_GATEWAY_TIMEOUT to avoid duplicate gateway calls",
		)
	}

	if cfg.GatewayTimeout*3 >= order.StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY_TIMEOUT (%s) is too large: 3× it must stay below the order stale-processing threshold (%s) so a valid PAYMENT_JOB_TIMEOUT range exists",
			cfg.GatewayTimeout,
			order.StaleProcessingThreshold,
		)
	}

	return cfg, nil
}
