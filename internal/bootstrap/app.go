package bootstrap

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	cartpg "github.com/residwi/go-api-project-template/internal/modules/cart/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	categorypg "github.com/residwi/go-api-project-template/internal/modules/category/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/checkout"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	dashboardpg "github.com/residwi/go-api-project-template/internal/modules/dashboard/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	channellog "github.com/residwi/go-api-project-template/internal/modules/notification/adapter/channel/log"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	reviewpg "github.com/residwi/go-api-project-template/internal/modules/review/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	shippingpg "github.com/residwi/go-api-project-template/internal/modules/shipping/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	userpg "github.com/residwi/go-api-project-template/internal/modules/user/adapter/postgres"
	userredis "github.com/residwi/go-api-project-template/internal/modules/user/adapter/redis"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	wishlistpg "github.com/residwi/go-api-project-template/internal/modules/wishlist/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	jobspg "github.com/residwi/go-api-project-template/internal/platform/jobs/postgres"
)

type App struct {
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
	TxRunner      database.TxRunner
}

func New(
	authCfg auth.Config,
	cartCfg cart.Config,
	paymentCfg payment.Config,
	db database.DB,
	cache *redis.Client,
	logger *slog.Logger,
) (*App, error) {
	txRunner := database.NewTxRunner(db.Primary)

	inv := inventory.New(inventorypg.New(db))
	prod := product.New(productpg.New(db), inv)
	categoryMod := category.New(categorypg.New(db), prod)
	promotionMod := promotion.New(promotionpg.New(db), txRunner)
	notificationMod := notification.New(
		notificationpg.New(db),
		txRunner,
		jobspg.New(db),
		channellog.New(logger),
		logger,
	)

	var statusCache user.StatusCache = user.NoCache{}
	if cache != nil {
		statusCache = userredis.New(cache)
	}
	userMod := user.New(userpg.New(db), statusCache, logger)
	authMod := auth.New(authCfg, userMod)

	cartMod := cart.New(cartpg.New(db), txRunner, prod, cartCfg.MaxItems)

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
		paymentCfg,
		logger,
		jobspg.New(db),
		ordMod,
		inv,
		promotionMod,
	)

	checkoutSvc := checkout.New(ordMod, paymentMod, logger)

	shippingMod := shipping.New(shippingpg.New(db), txRunner, ordMod)
	reviewMod := review.New(reviewpg.New(db), ordMod)

	return &App{
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
		TxRunner:      txRunner,
	}, nil
}
