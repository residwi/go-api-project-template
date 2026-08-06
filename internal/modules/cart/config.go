package cart

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config is cart's own configuration: the per-cart item cap.
type Config struct {
	MaxItems int `envconfig:"MAX_CART_ITEMS" default:"50"`
}

// LoadConfig has no invariant to check: any non-negative MaxItems is a valid
// cap, and envconfig's own required/type checks already cover the rest.
func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading cart config: %w", err)
	}

	return cfg, nil
}
