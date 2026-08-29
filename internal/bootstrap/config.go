package bootstrap

import (
	"fmt"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/config"
)

type Config struct {
	Auth         auth.Config
	Cart         cart.Config
	Notification notification.Config
	Order        order.Config
	Payment      payment.Config
}

func LoadConfig(appCfg *config.Settings) (Config, error) {
	var cfg Config

	authCfg, err := auth.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading auth config: %w", err)
	}

	cartCfg, err := cart.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading cart config: %w", err)
	}

	notificationCfg, err := notification.LoadConfig()
	if err != nil {
		return cfg, fmt.Errorf("loading notification config: %w", err)
	}

	paymentCfg, err := payment.LoadConfig(appCfg.App.Env)
	if err != nil {
		return cfg, fmt.Errorf("loading payment config: %w", err)
	}

	orderCfg, err := order.LoadConfig(paymentCfg.JobTimeout)
	if err != nil {
		return cfg, fmt.Errorf("loading order config: %w", err)
	}

	return Config{
		Auth:         authCfg,
		Cart:         cartCfg,
		Notification: notificationCfg,
		Order:        orderCfg,
		Payment:      paymentCfg,
	}, nil
}
