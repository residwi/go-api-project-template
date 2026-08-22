package bootstrap

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/modules/checkout"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Auth       auth.Config
	Cart       cart.Config
	Payment    payment.Config
	Pool       *pgxpool.Pool
	ReaderPool *pgxpool.Pool
	Cache      *redis.Client
	Logger     *slog.Logger
}

type App struct {
	Users         *user.Module
	Auth          *auth.Module
	Categories    *category.Module
	Products      *product.Module
	Inventory     *inventory.Module
	Carts         *cart.Module
	Orders        *order.Module
	Payments      *payment.Module
	Checkout      *checkout.Service
	Shipping      *shipping.Module
	Reviews       *review.Module
	Promotions    *promotion.Module
	Wishlists     *wishlist.Module
	Notifications *notification.Module
	Dashboard     *dashboard.Module
	TxRunner      database.TxRunner
}

func New(d Deps) (*App, error) {
	txRunner := database.NewTxRunner(d.Pool)

	inv := inventory.New(inventory.Deps{Pool: d.Pool})
	prod := product.New(product.Deps{Pool: d.Pool, InventoryReader: inv.Query, InventoryRegistrar: inv.Register})
	categoryMod := category.New(category.Deps{Pool: d.Pool, Products: prod.Query})
	promotionMod := promotion.New(promotion.Deps{Pool: d.Pool, Tx: txRunner})
	notificationMod := notification.New(notification.Deps{Pool: d.Pool, Logger: d.Logger})

	userMod := user.New(user.Deps{Pool: d.Pool, Cache: d.Cache, Logger: d.Logger})
	authMod := auth.New(auth.Deps{Config: d.Auth, Users: userMod.Credentials})

	cartMod := cart.New(cart.Deps{Pool: d.Pool, Tx: txRunner, MaxItems: d.Cart.MaxItems, Products: prod.Query})

	ordMod := order.New(order.Deps{
		Pool: d.Pool, Tx: txRunner, Logger: d.Logger,
		CartLock:         cartMod.Lock,
		CartRead:         cartMod.Query,
		CartClear:        cartMod.ClearCart,
		InventoryReserve: inv.Reserve,
		InventoryDeduct:  inv.Deduct,
		InventoryRestore: inv.Restore,
		Promotions:       promotionMod.Reserve,
		Notifications:    notificationMod.Jobs,
	})

	paymentMod := payment.New(payment.Deps{
		Pool:             d.Pool,
		Tx:               txRunner,
		Config:           d.Payment,
		Logger:           d.Logger,
		OrderTransition:  ordMod.Transition,
		OrderCanceller:   ordMod.Cancel,
		OrderReader:      ordMod.Query,
		InventoryDeduct:  inv.Deduct,
		InventoryRestore: inv.Restore,
		Promotions:       promotionMod.Reserve,
	})

	checkoutSvc := checkout.New(checkout.Deps{
		Orders:      ordMod.Place,
		Snapshots:   ordMod.Query,
		Cancels:     ordMod.Cancel,
		Payments:    paymentMod.Charge,
		PaymentJobs: paymentMod.Jobs,
		Logger:      d.Logger,
	})

	shippingMod := shipping.New(shipping.Deps{
		Pool: d.Pool, Tx: txRunner,
		OrderRead:        ordMod.Query,
		OrderStatusWrite: ordMod.Transition,
	})
	reviewMod := review.New(review.Deps{Pool: d.Pool, Purchase: ordMod.Query})

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
		Wishlists:     wishlist.New(wishlist.Deps{Pool: d.Pool}),
		Notifications: notificationMod,
		Dashboard:     dashboard.New(dashboard.Deps{Pool: d.Pool}),
		TxRunner:      txRunner,
	}, nil
}
