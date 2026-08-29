package notification

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	JobInterval    time.Duration `envconfig:"NOTIFICATION_JOB_INTERVAL"    default:"5s"`
	JobConcurrency int           `envconfig:"NOTIFICATION_JOB_CONCURRENCY" default:"10"`
	JobTimeout     time.Duration `envconfig:"NOTIFICATION_JOB_TIMEOUT"     default:"30s"`
}

const minJobInterval = 5 * time.Second

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading notification config: %w", err)
	}

	if cfg.JobInterval < minJobInterval {
		return Config{}, errors.New(
			"NOTIFICATION_JOB_INTERVAL must be at least 5s to avoid database polling overhead",
		)
	}

	if cfg.JobConcurrency < 1 {
		return Config{}, errors.New(
			"NOTIFICATION_JOB_CONCURRENCY must be at least 1 (0 deadlocks the worker on its unbuffered semaphore)",
		)
	}

	return cfg, nil
}
