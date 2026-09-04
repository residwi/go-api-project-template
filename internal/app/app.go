package app

import (
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/auth"
	authjwt "github.com/residwi/go-api-project-template/internal/features/auth/adapter/jwt"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	cartpg "github.com/residwi/go-api-project-template/internal/features/cart/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/category"
	categorypg "github.com/residwi/go-api-project-template/internal/features/category/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/checkout"
	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	dashboardpg "github.com/residwi/go-api-project-template/internal/features/dashboard/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/features/inventory/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	channellog "github.com/residwi/go-api-project-template/internal/features/notification/adapter/channel/log"
	notificationjobs "github.com/residwi/go-api-project-template/internal/features/notification/adapter/jobs"
	notificationpg "github.com/residwi/go-api-project-template/internal/features/notification/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/order"
	orderpg "github.com/residwi/go-api-project-template/internal/features/order/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/stripe"
	paymentjobs "github.com/residwi/go-api-project-template/internal/features/payment/adapter/jobs"
	paymentpg "github.com/residwi/go-api-project-template/internal/features/payment/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/product"
	productpg "github.com/residwi/go-api-project-template/internal/features/product/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/features/promotion/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/review"
	reviewpg "github.com/residwi/go-api-project-template/internal/features/review/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	shippingpg "github.com/residwi/go-api-project-template/internal/features/shipping/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/user"
	userpg "github.com/residwi/go-api-project-template/internal/features/user/adapter/postgres"
	userredis "github.com/residwi/go-api-project-template/internal/features/user/adapter/redis"
	"github.com/residwi/go-api-project-template/internal/features/wishlist"
	wishlistpg "github.com/residwi/go-api-project-template/internal/features/wishlist/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobqueue"
)

type Services struct {
	Users         *user.Service
	Auth          *auth.Service
	Categories    *category.Service
	Products      *product.Service
	Inventory     *inventory.Service
	Carts         *cart.Service
	Orders        *order.Service
	Payments      *payment.Service
	Checkout      *checkout.Service
	Shipping      *shipping.Service
	Reviews       *review.Service
	Promotions    *promotion.Service
	Wishlists     *wishlist.Service
	Notifications *notification.Service
	Dashboard     *dashboard.Service
}

func New(
	cfg Config,
	db database.DB,
	cache *redis.Client,
	logger *slog.Logger,
) (*Services, error) {
	txRunner := database.NewTxRunner(db.Primary)

	insertClient, err := jobqueue.NewInsertClient(db)
	if err != nil {
		return nil, fmt.Errorf("building job insert client: %w", err)
	}

	inv := inventory.New(inventorypg.New(db))
	prod := product.New(productpg.New(db), inv)
	categoryMod := category.New(categorypg.New(db), prod)
	promotionMod := promotion.New(promotionpg.New(db), txRunner)
	notificationMod := notification.New(
		notificationpg.New(db),
		txRunner,
		notificationjobs.NewJobQueue(insertClient, db),
		channellog.New(logger),
		logger,
	)

	var statusCache user.StatusCache = user.NoCache{}
	if cache != nil {
		statusCache = userredis.New(cache)
	}
	userMod := user.New(userpg.New(db), statusCache, logger)
	authMod := auth.New(cfg.Auth, userMod, authjwt.New(cfg.Auth.Secret, cfg.Auth.Issuer))

	cartMod := cart.New(cartpg.New(db), txRunner, prod, cfg.Cart.MaxItems)

	ordMod := order.New(
		orderpg.New(db),
		txRunner,
		logger,
		cartMod,
		inv,
		promotionMod,
		notificationMod,
	)

	paymentMod := payment.New(
		paymentpg.New(db),
		txRunner,
		cfg.Payment,
		logger,
		newPaymentGateway(cfg.Payment),
		paymentjobs.NewJobQueue(insertClient, db),
		ordMod,
		inv,
		promotionMod,
	)

	checkoutSvc := checkout.New(ordMod, paymentMod, logger)

	shippingMod := shipping.New(shippingpg.New(db), txRunner, ordMod)
	reviewMod := review.New(reviewpg.New(db), ordMod)

	return &Services{
		Users:         userMod,
		Auth:          authMod,
		Categories:    categoryMod,
		Products:      prod,
		Inventory:     inv,
		Carts:         cartMod,
		Orders:        ordMod,
		Payments:      paymentMod,
		Checkout:      checkoutSvc,
		Shipping:      shippingMod,
		Reviews:       reviewMod,
		Promotions:    promotionMod,
		Wishlists:     wishlist.New(wishlistpg.New(db)),
		Notifications: notificationMod,
		Dashboard:     dashboard.New(dashboardpg.New(db)),
	}, nil
}

func newPaymentGateway(cfg payment.Config) payment.Gateway {
	switch cfg.Gateway {
	case payment.GatewayStripe:
		return gatewaystripe.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	case payment.GatewayMidtrans:
		return gatewaymidtrans.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	default:
		return gatewaymock.New(cfg.GatewayURL, cfg.GatewayTimeout)
	}
}
