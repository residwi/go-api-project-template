package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Secret          string        `envconfig:"JWT_SECRET"       required:"true"`
	Issuer          string        `envconfig:"JWT_ISSUER"                       default:"ecommerce-api"`
	AccessTokenTTL  time.Duration `envconfig:"JWT_ACCESS_TTL"                   default:"15m"`
	RefreshTokenTTL time.Duration `envconfig:"JWT_REFRESH_TTL"                  default:"168h"`
	RateLimit       int           `envconfig:"AUTH_RATE_LIMIT"                  default:"10"`
	RateWindow      time.Duration `envconfig:"AUTH_RATE_WINDOW"                 default:"1m"`
	BcryptCost      int           `envconfig:"BCRYPT_COST"                      default:"10"`
}

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading auth config: %w", err)
	}

	if cfg.RateWindow < time.Second {
		return Config{}, errors.New(
			"AUTH_RATE_WINDOW must be at least 1s (sub-second windows divide by zero in the limiter)",
		)
	}

	if cfg.BcryptCost < bcrypt.MinCost || cfg.BcryptCost > bcrypt.MaxCost {
		return Config{}, fmt.Errorf(
			"BCRYPT_COST must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost,
		)
	}

	return cfg, nil
}
