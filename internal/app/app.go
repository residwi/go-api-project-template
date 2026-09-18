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
	categoryredis "github.com/residwi/go-api-project-template/internal/features/category/adapter/redis"
	"github.com/residwi/go-api-project-template/internal/features/checkout"
	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	dashboardpg "github.com/residwi/go-api-project-template/internal/features/dashboard/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/features/inventory/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	channellog "github.com/residwi/go-api-project-template/internal/features/notification/adapter/channel/log"
	notificationjobs "github.com/residwi/go-api-project-template/internal/features/notification/adapter/jobs"
	notificationpg "github.com/residwi/go-api-project-template/internal/features/notification/adapter/postgres"
	notificationredis "github.com/residwi/go-api-project-template/internal/features/notification/adapter/redis"
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
	productredis "github.com/residwi/go-api-project-template/internal/features/product/adapter/redis"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/features/promotion/adapter/postgres"
	promotionredis "github.com/residwi/go-api-project-template/internal/features/promotion/adapter/redis"
	"github.com/residwi/go-api-project-template/internal/features/review"
	reviewpg "github.com/residwi/go-api-project-template/internal/features/review/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	shippingpg "github.com/residwi/go-api-project-template/internal/features/shipping/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/features/user"
	userpg "github.com/residwi/go-api-project-template/internal/features/user/adapter/postgres"
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

	var categoryRepo category.Repository = categorypg.New(db)
	var productRepo product.Repository = productpg.New(db)
	var promotionRepo promotion.Repository = promotionpg.New(db)
	var notificationRepo notification.Repository = notificationpg.New(db)

	if cache != nil {
		categoryRepo = categoryredis.New(categoryRepo, cache, logger)
		productRepo = productredis.New(productRepo, cache, logger)
		promotionRepo = promotionredis.New(promotionRepo, cache, logger)
		notificationRepo = notificationredis.New(notificationRepo, cache, logger)
	}

	inv := inventory.New(inventorypg.New(db))
	prod := product.New(productRepo, inv)
	categoryMod := category.New(categoryRepo, prod)
	promotionMod := promotion.New(promotionRepo, txRunner)
	notificationMod := notification.New(
		notificationRepo,
		txRunner,
		notificationjobs.NewJobQueue(insertClient, db),
		channellog.New(logger),
		logger,
	)

	userMod := user.New(userpg.New(db))
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
