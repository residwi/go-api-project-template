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
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
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
	Users         *user.Module
	Auth          *auth.Module
	Categories    *category.Module
	Products      *product.Module
	Inventory     *inventory.Module
	Carts         *cart.Service
	Orders        *order.Service
	Payments      *payment.Service
	Shipping      *shipping.Module
	Reviews       *review.Module
	Promotions    *promotion.Module
	Wishlists     *wishlist.Module
	Notifications *notification.Module
	Dashboard     *dashboard.Module
	TxRunner      database.TxRunner
	Gateway       payment.Gateway
}

// New builds every service. Cache may be nil: user's status cache degrades to
// query.NoCache rather than failing the boot.
func New(d Deps) (*App, error) {
	txRunner := database.NewTxRunner(d.Pool)

	inv := inventory.New(inventory.Deps{Pool: d.Pool})
	// inv.Query satisfies product's query and images InventoryReader ports, and
	// inv.Register satisfies create's InventoryRegistrar port, all by name-match.
	prod := product.New(product.Deps{Pool: d.Pool, InventoryReader: inv.Query, InventoryRegistrar: inv.Register})
	// prod.Query satisfies remove.ProductCounter by name-match.
	categoryMod := category.New(category.Deps{Pool: d.Pool, Products: prod.Query})
	promotionMod := promotion.New(promotion.Deps{Pool: d.Pool, Tx: txRunner})
	notificationMod := notification.New(notification.Deps{Pool: d.Pool, Logger: d.Logger})

	userMod := user.New(user.Deps{Pool: d.Pool, Cache: d.Cache, Logger: d.Logger})
	// userMod.Credentials satisfies auth.UserPorts by name-match.
	authMod := auth.New(auth.Deps{Config: d.Auth, Users: userMod.Credentials})

	// prod.Query satisfies cart.ProductLookup by name-match.
	cartSvc := cart.NewService(cartpg.New(d.Pool), txRunner, prod.Query, d.Cart.MaxItems)

	orderSvc := order.NewService(
		orderpg.New(d.Pool), txRunner,
		cartSvc,  // CartProvider
		inv,      // InventoryReserver -- ReserveBatch, DeductBatch, Restore, by name-match
		nil, nil, // payment ports: the cycle, set below by SetOrderPaymentDeps
		promotionMod.Reserve, // CouponReserver
		notificationMod.Jobs, // NotificationEnqueuer -- EnqueueOrderPlaced by name-match
		d.Logger,
	)

	gw := mockgateway.New(d.Payment.GatewayURL, d.Payment.GatewayTimeout)
	paymentSvc := payment.NewService(
		paymentpg.New(d.Pool), txRunner, gw,
		orderSvc, // OrderUpdater, OrderGetter, OrderItemsGetter -- all by name-match
		orderSvc,
		orderSvc,
		inv, // InventoryDeductor, InventoryRestorer -- DeductBatch and Restore, by name-match
		inv,
		promotionMod.Reserve, // CouponReleaser
		d.Logger,
	)
	SetOrderPaymentDeps(orderSvc, paymentSvc)

	// orderSvc satisfies shipping.OrderPorts and create.PurchaseVerifier directly,
	// so neither needs a bootstrap adapter.
	shippingMod := shipping.New(shipping.Deps{Pool: d.Pool, Tx: txRunner, Orders: orderSvc})
	reviewMod := review.New(review.Deps{Pool: d.Pool, Purchase: orderSvc})

	return &App{
		Users:         userMod,
		Auth:          authMod,
		Categories:    categoryMod,
		Products:      prod,
		Inventory:     inv,
		Carts:         cartSvc,
		Orders:        orderSvc,
		Payments:      paymentSvc,
		Shipping:      shippingMod,
		Reviews:       reviewMod,
		Promotions:    promotionMod,
		Wishlists:     wishlist.New(wishlist.Deps{Pool: d.Pool}),
		Notifications: notificationMod,
		Dashboard:     dashboard.New(dashboard.Deps{Pool: d.Pool}),
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
