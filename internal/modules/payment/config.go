package payment

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

// Config is payment's own configuration. It loads itself so that no central
// struct has to know payment's env vars, and validates the invariants payment
// is the one to suffer from.
type Config struct {
	Gateway        string        `envconfig:"PAYMENT_GATEWAY"         default:"mock"`
	GatewayURL     string        `envconfig:"PAYMENT_GATEWAY_URL"     default:"http://localhost:8080/mock/payment"`
	GatewayTimeout time.Duration `envconfig:"PAYMENT_GATEWAY_TIMEOUT" default:"10s"`
	GatewayAPIKey  string        `envconfig:"PAYMENT_GATEWAY_API_KEY" default:""`
	WebhookSecret  string        `envconfig:"PAYMENT_WEBHOOK_SECRET"  default:"webhook-secret"`
}

// defaultWebhookSecret is the placeholder; it must be overridden outside
// development.
const defaultWebhookSecret = "webhook-secret"

// LoadConfig takes appEnv and jobsLease because payment's invariants span them:
// the placeholder secret is only tolerable in development, and a lease shorter
// than three gateway timeouts lets the runner re-claim a charge that is still
// running, which calls the gateway twice for one order.
func LoadConfig(appEnv string, jobsLease time.Duration) (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading payment config: %w", err)
	}

	if appEnv != "development" && cfg.WebhookSecret == defaultWebhookSecret {
		return Config{}, errors.New("PAYMENT_WEBHOOK_SECRET must be set in non-development environments")
	}

	if jobsLease < cfg.GatewayTimeout*3 {
		return Config{}, errors.New(
			"WORKER_LEASE_DURATION must be at least 3× PAYMENT_GATEWAY_TIMEOUT to avoid duplicate gateway calls",
		)
	}

	// If 3x the gateway timeout already meets the stale threshold, no lease
	// duration satisfies both bounds: name that instead of a confusing lease error.
	if cfg.GatewayTimeout*3 >= ordercontract.StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY_TIMEOUT (%s) is too large: 3× it must stay below the order stale-processing threshold (%s) so a valid WORKER_LEASE_DURATION range exists",
			cfg.GatewayTimeout,
			ordercontract.StaleProcessingThreshold,
		)
	}

	return cfg, nil
}
