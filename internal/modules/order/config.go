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

	JobInterval    time.Duration `envconfig:"ORDER_JOB_INTERVAL"    default:"1m"`
	JobConcurrency int           `envconfig:"ORDER_JOB_CONCURRENCY" default:"1"`
	JobLease       time.Duration `envconfig:"ORDER_JOB_LEASE"       default:"2m"`
}

const minJobInterval = 5 * time.Second

func LoadConfig(paymentJobLease time.Duration) (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading order config: %w", err)
	}

	if paymentJobLease >= StaleProcessingThreshold {
		return Config{}, fmt.Errorf(
			"PAYMENT_JOB_LEASE (%s) must be less than the order stale-processing threshold (%s), or the recovery sweep can revert an order whose charge is still leased",
			paymentJobLease,
			StaleProcessingThreshold,
		)
	}

	if cfg.RateWindow < time.Second {
		return Config{}, errors.New(
			"ORDER_RATE_WINDOW must be at least 1s (sub-second windows divide by zero in the limiter)",
		)
	}

	if cfg.JobInterval < minJobInterval {
		return Config{}, errors.New("ORDER_JOB_INTERVAL must be at least 5s to avoid database polling overhead")
	}

	if cfg.JobConcurrency < 1 {
		return Config{}, errors.New(
			"ORDER_JOB_CONCURRENCY must be at least 1 (0 deadlocks the worker on its unbuffered semaphore)",
		)
	}

	return cfg, nil
}
