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
	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	ordercancel "github.com/residwi/go-api-project-template/internal/modules/order/cancel"
	ordercancelpg "github.com/residwi/go-api-project-template/internal/modules/order/cancel/postgres"
	ordertransition "github.com/residwi/go-api-project-template/internal/modules/order/transition"
	ordertransitionpg "github.com/residwi/go-api-project-template/internal/modules/order/transition/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	"github.com/residwi/go-api-project-template/internal/platform/database"

	orderquery "github.com/residwi/go-api-project-template/internal/modules/order/query"
	orderquerypg "github.com/residwi/go-api-project-template/internal/modules/order/query/postgres"
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
	Carts         *cart.Module
	Orders        *order.Module
	Payments      *payment.Module
	Shipping      *shipping.Module
	Reviews       *review.Module
	Promotions    *promotion.Module
	Wishlists     *wishlist.Module
	Notifications *notification.Module
	Dashboard     *dashboard.Module
	TxRunner      database.TxRunner
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

	// prod.Query satisfies cart.ProductPorts by name-match.
	cartMod := cart.New(cart.Deps{Pool: d.Pool, Tx: txRunner, MaxItems: d.Cart.MaxItems, Products: prod.Query})

	// order/transition and order/query need nothing but the pool, so a second,
	// throwaway pair can be built here purely for payment to read from -- both
	// are stateless wrappers over the same orders/order_items tables order.Module
	// builds its own copy of below, so two instances behave identically to one
	// shared instance. That is what removes the order<->payment setter: at slice
	// granularity the cycle runs through four packages, not two, so payment can
	// be built before order needs anything back from it.
	//
	// order/cancel gets the same treatment, for CancelUnpaid alone: unlike a bare
	// Mark*, order.Module's own Cancel command needs payment.Jobs as its
	// paymentCancel dependency, which does not exist yet either. CancelUnpaid
	// itself never reads that dependency (only the user-triggered Execute path
	// does), so this second command is built with a nil one and is safe to call
	// for CancelUnpaid only.
	orderTransition := ordertransition.New(ordertransitionpg.New(d.Pool))
	orderQuery := orderquery.New(orderquerypg.New(d.Pool))
	orderCanceller := ordercancel.New(
		ordercancelpg.New(d.Pool), txRunner, orderTransition, inv, promotionMod.Reserve, nil, d.Logger,
	)

	paymentMod := payment.New(payment.Deps{
		Pool:            d.Pool,
		Tx:              txRunner,
		Config:          d.Payment,
		Logger:          d.Logger,
		OrderTransition: orderTransition, // order.Mark* -- by name-match
		OrderCanceller:  orderCanceller,  // CancelUnpaid -- by name-match
		OrderReader:     orderQuery,      // GetSnapshot, ListItemQuantities -- by name-match
		Inventory:       inv,             // DeductBatch, Restore -- by name-match
		Promotions:      promotionMod.Reserve,
	})

	ordMod := order.New(order.Deps{
		Pool: d.Pool, Tx: txRunner, Logger: d.Logger,
		Cart:          cartMod,              // CartProvider -- LockCart, GetSnapshot, Clear, by name-match
		Inventory:     inv,                  // InventoryPort -- ReserveBatch, DeductBatch, Restore, by name-match
		Promotions:    promotionMod.Reserve, // CouponPort -- Reserve, Release, by name-match
		Notifications: notificationMod.Jobs, // NotificationEnqueuer -- EnqueueOrderPlaced by name-match
		Payment:       paymentMod.Charge,    // PaymentInitiator -- InitiatePayment, by name-match
		PaymentJobs:   paymentMod.Jobs,      // PaymentJobCanceller -- CancelPendingByOrderID, by name-match
	})

	// ordMod satisfies shipping.OrderPorts and create.PurchaseVerifier directly,
	// so neither needs a bootstrap adapter.
	shippingMod := shipping.New(shipping.Deps{Pool: d.Pool, Tx: txRunner, Orders: ordMod})
	reviewMod := review.New(review.Deps{Pool: d.Pool, Purchase: ordMod})

	return &App{
		Users:         userMod,
		Auth:          authMod,
		Categories:    categoryMod,
		Products:      prod,
		Inventory:     inv,
		Carts:         cartMod,
		Orders:        ordMod,
		Payments:      paymentMod,
		Shipping:      shippingMod,
		Reviews:       reviewMod,
		Promotions:    promotionMod,
		Wishlists:     wishlist.New(wishlist.Deps{Pool: d.Pool}),
		Notifications: notificationMod,
		Dashboard:     dashboard.New(dashboard.Deps{Pool: d.Pool}),
		TxRunner:      txRunner,
	}, nil
}
