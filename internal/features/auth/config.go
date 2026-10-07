package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Secret          string        `envconfig:"JWT_SECRET"       required:"true"`
	Issuer          string        `envconfig:"JWT_ISSUER"                       default:"ecommerce-api"`
	AccessTokenTTL  time.Duration `envconfig:"JWT_ACCESS_TTL"                   default:"15m"`
	RefreshTokenTTL time.Duration `envconfig:"JWT_REFRESH_TTL"                  default:"168h"`
	RateLimit       int           `envconfig:"AUTH_RATE_LIMIT"                  default:"10"`
	RateWindow      time.Duration `envconfig:"AUTH_RATE_WINDOW"                 default:"1m"`
}

// minSecretBytes floors the HS256 key at 256 bits, matching the HMAC-SHA256 output width.
const minSecretBytes = 32

const placeholderSecret = "your-secret-key-change-in-production"

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading auth config: %w", err)
	}

	if cfg.Secret == placeholderSecret {
		return Config{}, errors.New(
			"JWT_SECRET must not be the .env.example placeholder value; set a unique secret",
		)
	}

	if len(cfg.Secret) < minSecretBytes {
		return Config{}, fmt.Errorf(
			"JWT_SECRET must be at least %d bytes", minSecretBytes,
		)
	}

	if cfg.RateWindow < time.Second {
		return Config{}, errors.New(
			"AUTH_RATE_WINDOW must be at least 1s (a deliberate minimum window)",
		)
	}

	return cfg, nil
}
