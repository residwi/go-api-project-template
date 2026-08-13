package payment

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

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

// The three gateways newGateway (module.go) knows how to build. Named here,
// not just there, so LoadConfig can reject an unrecognised Config.Gateway at
// boot instead of newGateway silently falling back to the mock on a typo --
// the same invariant, checked where a wrong value is loud instead of where a
// wrong value would otherwise stay quiet until every real charge fails or,
// worse, a reachable dev mock gateway fakes one through unpaid.
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
			"WORKER_LEASE_DURATION must be at least 3× PAYMENT_GATEWAY_TIMEOUT to avoid duplicate gateway calls",
		)
	}

	if cfg.GatewayTimeout*3 >= ordercontract.StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"PAYMENT_GATEWAY_TIMEOUT (%s) is too large: 3× it must stay below the order stale-processing threshold (%s) so a valid WORKER_LEASE_DURATION range exists",
			cfg.GatewayTimeout,
			ordercontract.StaleProcessingThreshold,
		)
	}

	return cfg, nil
}
