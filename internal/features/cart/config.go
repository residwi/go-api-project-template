package cart

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	MaxItems int `envconfig:"MAX_CART_ITEMS" default:"50"`
}

func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("loading cart config: %w", err)
	}

	return cfg, nil
}
