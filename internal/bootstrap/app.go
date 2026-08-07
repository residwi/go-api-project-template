// Package bootstrap is the composition root. It constructs every repository and
// service and wires the cross-module ports together. After phase 0 it holds no
// adapters: every port is satisfied by a service method of the same name, or by
// a type from a module's contract package.
package bootstrap

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	cartpg "github.com/residwi/go-api-project-template/internal/modules/cart/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	categorypg "github.com/residwi/go-api-project-template/internal/modules/category/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	dashboardpg "github.com/residwi/go-api-project-template/internal/modules/dashboard/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	reviewpg "github.com/residwi/go-api-project-template/internal/modules/review/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	userpg "github.com/residwi/go-api-project-template/internal/modules/user/postgres"
	userredis "github.com/residwi/go-api-project-template/internal/modules/user/redis"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	wishlistpg "github.com/residwi/go-api-project-template/internal/modules/wishlist/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// Deps is what New needs to build every service: infrastructure connections
// plus each module's own typed config (auth.Config, cart.Config,
// payment.Config), loaded upstream by that module's own LoadConfig. There is
// no *config.Settings field here -- New never read one, and JWT/cart/payment
// values now live on the modules that declare them rather than on a shared
// struct New had to reach into.
type Deps struct {
	Auth       auth.Config
	Cart       cart.Config
	Payment    payment.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	Cache      *redis.Client
	Logger     *slog.Logger
}

// App is every wired service, exported for the router and worker binaries to
// register routes and jobs against.
type App struct {
	Users         *user.Service
	Auth          *auth.Service
	Categories    *category.Service
	Products      *product.Service
	Inventory     *inventory.Service
	Carts         *cart.Service
	Orders        *order.Service
	Payments      *payment.Service
	Shipping      *shipping.Module
	Reviews       *review.Service
	Promotions    *promotion.Service
	Wishlists     *wishlist.Service
	Notifications *notification.Service
	Dashboard     *dashboard.Service
	TxRunner      database.TxRunner
	Gateway       payment.Gateway
}

// New builds every service. Cache may be nil: user's status cache degrades to
// user.NoCache rather than failing the boot.
func New(d Deps) (*App, error) {
	txRunner := database.NewTxRunner(d.Pool)

	var userCache user.StatusCache = user.NoCache{}
	if d.Cache != nil {
		userCache = userredis.New(d.Cache)
	}

	inventorySvc := inventory.NewService(inventorypg.New(d.Pool))
	// inventorySvc satisfies both product.InventoryReader and
	// product.InventoryRegistrar by name-match.
	productSvc := product.NewService(productpg.New(d.Pool), inventorySvc, inventorySvc)
	categorySvc := category.NewService(categorypg.New(d.Pool), productSvc)
	promotionSvc := promotion.NewService(promotionpg.New(d.Pool), txRunner)
	notificationSvc := notification.NewService(notificationpg.New(d.Pool), d.Logger)

	userSvc := user.NewService(userpg.New(d.Pool), userCache, d.Logger)
	authSvc := auth.NewService(
		userSvc,
		d.Auth.Secret,
		d.Auth.Issuer,
		d.Auth.AccessTokenTTL,
		d.Auth.RefreshTokenTTL,
	)
	authSvc.SetBcryptCost(d.Auth.BcryptCost)

	cartSvc := cart.NewService(cartpg.New(d.Pool), txRunner, productSvc, d.Cart.MaxItems)

	orderSvc := order.NewService(
		orderpg.New(d.Pool), txRunner,
		cartSvc,      // CartProvider
		inventorySvc, // InventoryReserver
		nil, nil,     // payment ports: the cycle, set below by SetOrderPaymentDeps
		promotionSvc, // CouponReserver
		notificationSvc,
		d.Logger,
	)

	gw := mockgateway.New(d.Payment.GatewayURL, d.Payment.GatewayTimeout)
	paymentSvc := payment.NewService(
		paymentpg.New(d.Pool), txRunner, gw,
		orderSvc, // OrderUpdater, OrderGetter, OrderItemsGetter -- all by name-match
		orderSvc,
		orderSvc,
		inventorySvc, // InventoryDeductor, InventoryRestorer
		inventorySvc,
		promotionSvc, // CouponReleaser
		d.Logger,
	)
	SetOrderPaymentDeps(orderSvc, paymentSvc)

	// orderSvc satisfies shipping.OrderPorts and review.PurchaseVerifier directly,
	// so neither needs a bootstrap adapter.
	shippingMod := shipping.New(shipping.Deps{Pool: d.Pool, Tx: txRunner, Orders: orderSvc})
	reviewSvc := review.NewService(reviewpg.New(d.Pool), orderSvc)

	return &App{
		Users:         userSvc,
		Auth:          authSvc,
		Categories:    categorySvc,
		Products:      productSvc,
		Inventory:     inventorySvc,
		Carts:         cartSvc,
		Orders:        orderSvc,
		Payments:      paymentSvc,
		Shipping:      shippingMod,
		Reviews:       reviewSvc,
		Promotions:    promotionSvc,
		Wishlists:     wishlist.NewService(wishlistpg.New(d.Pool)),
		Notifications: notificationSvc,
		Dashboard:     dashboard.NewService(dashboardpg.New(d.Pool)),
		TxRunner:      txRunner,
		Gateway:       gw,
	}, nil
}

// SetOrderPaymentDeps breaks the order-payment cycle: at whole-service
// granularity each needs the other, so one of them must be wired after
// construction. Both ports are satisfied by paymentSvc directly.
func SetOrderPaymentDeps(orderSvc *order.Service, paymentSvc *payment.Service) {
	orderSvc.SetPaymentDeps(paymentSvc, paymentSvc)
}
