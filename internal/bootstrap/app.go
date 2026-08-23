package bootstrap

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
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

func New(d Deps) (*App, error) {
	txRunner := database.NewTxRunner(d.Pool)

	inv := inventory.New(inventory.Deps{Repo: inventorypg.New(d.Pool)})
	prod := product.New(
		product.Deps{Repo: productpg.New(d.Pool), InventoryReader: inv, InventoryRegistrar: inv},
	)
	categoryMod := category.New(category.Deps{Repo: categorypg.New(d.Pool), Products: prod})
	promotionMod := promotion.New(promotion.Deps{Repo: promotionpg.New(d.Pool), Tx: txRunner})
	notificationMod := notification.New(notification.Deps{
		Repo:   notificationpg.New(d.Pool),
		Pool:   d.Pool,
		Logger: d.Logger,
	})

	var statusCache user.StatusCache = user.NoCache{}
	if d.Cache != nil {
		statusCache = userredis.New(d.Cache)
	}
	userMod := user.New(user.Deps{Repo: userpg.New(d.Pool), Cache: statusCache, Logger: d.Logger})
	authMod := auth.New(auth.Deps{Config: d.Auth, Users: userMod})

	cartMod := cart.New(cart.Deps{
		Repo: cartpg.New(d.Pool), Tx: txRunner, MaxItems: d.Cart.MaxItems, Products: prod,
	})

	ordMod := order.New(order.Deps{
		Repo: orderpg.New(d.Pool), Tx: txRunner, Logger: d.Logger,
		CartLock:         cartMod,
		CartRead:         cartMod,
		CartClear:        cartMod,
		InventoryReserve: inv,
		InventoryDeduct:  inv,
		InventoryRestore: inv,
		Promotions:       promotionMod,
		Notifications:    notificationMod.Jobs,
	})

	paymentMod := payment.New(payment.Deps{
		Repo:             paymentpg.New(d.Pool),
		Pool:             d.Pool,
		Tx:               txRunner,
		Config:           d.Payment,
		Logger:           d.Logger,
		OrderTransition:  ordMod,
		OrderCanceller:   ordMod,
		OrderReader:      ordMod,
		InventoryDeduct:  inv,
		InventoryRestore: inv,
		Promotions:       promotionMod,
	})

	checkoutSvc := checkout.New(checkout.Deps{
		Orders:      ordMod,
		Snapshots:   ordMod,
		Cancels:     ordMod,
		Payments:    paymentMod,
		PaymentJobs: paymentMod,
		Logger:      d.Logger,
	})

	shippingMod := shipping.New(shipping.Deps{
		Repo: shippingpg.New(d.Pool), Tx: txRunner,
		OrderRead:    ordMod,
		OrderShip:    ordMod,
		OrderDeliver: ordMod,
	})
	reviewMod := review.New(review.Deps{Repo: reviewpg.New(d.Pool), Purchase: ordMod})

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
