package promotion

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	RateLimit  int           `envconfig:"PROMOTION_RATE_LIMIT"  default:"10"`
	RateWindow time.Duration `envconfig:"PROMOTION_RATE_WINDOW" default:"1m"`
}

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading promotion config: %w", err)
	}

	if cfg.RateWindow < time.Second {
		return Config{}, errors.New(
			"PROMOTION_RATE_WINDOW must be at least 1s (a deliberate minimum window)",
		)
	}

	return cfg, nil
}
