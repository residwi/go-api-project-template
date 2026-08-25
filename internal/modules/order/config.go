package order

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	RateLimit  int           `envconfig:"ORDER_RATE_LIMIT"  default:"5"`
	RateWindow time.Duration `envconfig:"ORDER_RATE_WINDOW" default:"1m"`
}

func LoadConfig(jobsLease time.Duration) (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading order config: %w", err)
	}

	if jobsLease >= StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"WORKER_PAYMENT_LEASE (%s) must be less than the order stale-processing threshold (%s), or the recovery sweep can revert an order whose charge is still leased",
			jobsLease,
			StaleProcessingThreshold,
		)
	}

	if cfg.RateWindow < time.Second {
		return Config{}, errors.New(
			"ORDER_RATE_WINDOW must be at least 1s (sub-second windows divide by zero in the limiter)",
		)
	}

	return cfg, nil
}
