package payment

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Gateway        string        `envconfig:"PAYMENT_GATEWAY"         default:"mock"`
	GatewayURL     string        `envconfig:"PAYMENT_GATEWAY_URL"     default:"http://localhost:8080/mock/payment"`
	GatewayTimeout time.Duration `envconfig:"PAYMENT_GATEWAY_TIMEOUT" default:"10s"`
	GatewayAPIKey  string        `envconfig:"PAYMENT_GATEWAY_API_KEY" default:""`
	WebhookSecret  string        `envconfig:"PAYMENT_WEBHOOK_SECRET"`

	JobInterval    time.Duration `envconfig:"PAYMENT_JOB_INTERVAL"    default:"10s"`
	JobConcurrency int           `envconfig:"PAYMENT_JOB_CONCURRENCY" default:"5"`
	JobTimeout     time.Duration `envconfig:"PAYMENT_JOB_TIMEOUT"     default:"2m"`
}

const (
	GatewayMock     = "mock"
	GatewayStripe   = "stripe"
	GatewayMidtrans = "midtrans"
)

const defaultWebhookSecret = "webhook-secret"

// minWebhookSecretBytes floors the HMAC-SHA256 key at 256 bits, the digest width.
const minWebhookSecretBytes = 32

const minJobInterval = 5 * time.Second

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading payment config: %w", err)
	}

	if cfg.Gateway != GatewayMock && cfg.Gateway != GatewayStripe && cfg.Gateway != GatewayMidtrans {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY must be %q, %q or %q, got %q", GatewayMock, GatewayStripe, GatewayMidtrans, cfg.Gateway,
		)
	}

	if cfg.WebhookSecret == "" || cfg.WebhookSecret == defaultWebhookSecret {
		return Config{}, errors.New("PAYMENT_WEBHOOK_SECRET must be set to a non-default value")
	}

	if len(cfg.WebhookSecret) < minWebhookSecretBytes {
		return Config{}, fmt.Errorf("PAYMENT_WEBHOOK_SECRET must be at least %d bytes", minWebhookSecretBytes)
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

	return cfg, nil
}
