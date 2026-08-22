package bootstrap

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	categorypg "github.com/residwi/go-api-project-template/internal/modules/category/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/checkout"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	dashboardpg "github.com/residwi/go-api-project-template/internal/modules/dashboard/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/modules/review"
	reviewpg "github.com/residwi/go-api-project-template/internal/modules/review/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	shippingpg "github.com/residwi/go-api-project-template/internal/modules/shipping/adapter/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
	wishlistpg "github.com/residwi/go-api-project-template/internal/modules/wishlist/adapter/postgres"
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
	Auth          *auth.Service
	Categories    *category.Service
	Products      *product.Service
	Inventory     *inventory.Module
	Carts         *cart.Module
	Orders        *order.Module
	Payments      *payment.Module
	Checkout      *checkout.Service
	Shipping      *shipping.Service
	Reviews       *review.Service
	Promotions    *promotion.Module
	Wishlists     *wishlist.Service
	Notifications *notification.Service
	Dashboard     *dashboard.Service
	TxRunner      database.TxRunner
}

func New(d Deps) (*App, error) {
	txRunner := database.NewTxRunner(d.Pool)

	inv := inventory.New(inventory.Deps{Pool: d.Pool})
	prod := product.New(
		product.Deps{Repo: productpg.New(d.Pool), InventoryReader: inv.Query, InventoryRegistrar: inv.Register},
	)
	categoryMod := category.New(category.Deps{Repo: categorypg.New(d.Pool), Products: prod})
	promotionMod := promotion.New(promotion.Deps{Pool: d.Pool, Tx: txRunner})
	notificationMod := notification.New(notification.Deps{
		Repo:   notificationpg.New(d.Pool),
		Pool:   d.Pool,
		Logger: d.Logger,
	})

	userMod := user.New(user.Deps{Pool: d.Pool, Cache: d.Cache, Logger: d.Logger})
	authMod := auth.New(auth.Deps{Config: d.Auth, Users: userMod.Credentials})

	cartMod := cart.New(cart.Deps{Pool: d.Pool, Tx: txRunner, MaxItems: d.Cart.MaxItems, Products: prod})

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
		Repo: shippingpg.New(d.Pool), Tx: txRunner,
		OrderRead:    ordMod.Query,
		OrderShip:    ordMod.Transition,
		OrderDeliver: ordMod.Transition,
	})
	reviewMod := review.New(review.Deps{Repo: reviewpg.New(d.Pool), Purchase: ordMod.Query})

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
		Wishlists:     wishlist.New(wishlist.Deps{Repo: wishlistpg.New(d.Pool)}),
		Notifications: notificationMod,
		Dashboard:     dashboard.New(dashboard.Deps{Repo: dashboardpg.New(d.Pool)}),
		TxRunner:      txRunner,
	}, nil
}
