package order

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/cancel"
	cancelpg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/cancel/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/changestatus"
	changestatuspg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/changestatus/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/expire"
	expirepg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/expire/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/place"
	placepg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/place/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/recoverstale"
	recoverstalepg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/recoverstale/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/transition"
	transitionpg "github.com/residwi/go-api-project-template/internal/modules/order/usecase/transition/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Tx     database.TxRunner
	Logger *slog.Logger

	CartLock         CartLocker
	CartRead         CartReader
	CartClear        CartClearer
	InventoryReserve InventoryReserver
	InventoryDeduct  InventoryDeductor
	InventoryRestore InventoryRestorer
	Promotions       CouponPort
	Notifications    NotificationEnqueuer

	PaymentJobs PaymentJobCanceller
}

type CartLocker interface {
	Lock(ctx context.Context, userID uuid.UUID) error
}

type CartReader interface {
	Snapshot(ctx context.Context, userID uuid.UUID) (*cartcontract.Cart, error)
}

type CartClearer interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}

type InventoryReserver interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponPort interface {
	Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}

type Module struct {
	Place        *place.UseCase
	Query        *query.UseCase
	Cancel       *cancel.UseCase
	ChangeStatus *changestatus.UseCase
	Expire       *expire.UseCase
	RecoverStale *recoverstale.UseCase
	Transition   *transition.UseCase
}

func New(d Deps) *Module {
	transitionApplier := transition.New(transitionpg.New(d.Pool))

	return &Module{
		Place: place.New(place.Deps{
			Repo:          placepg.New(d.Pool),
			Tx:            d.Tx,
			Locker:        d.CartLock,
			Carts:         d.CartRead,
			Clearer:       d.CartClear,
			Reserver:      d.InventoryReserve,
			Deductor:      d.InventoryDeduct,
			Coupons:       d.Promotions,
			Notifications: d.Notifications,
			Transition:    transitionApplier,
			Logger:        d.Logger,
		}),
		Query: query.New(querypg.New(d.Pool)),
		Cancel: cancel.New(
			cancelpg.New(d.Pool), d.Tx, transitionApplier, d.InventoryRestore, d.Promotions, d.PaymentJobs, d.Logger,
		),
		ChangeStatus: changestatus.New(changestatuspg.New(d.Pool), transitionApplier),
		Expire: expire.New(
			expirepg.New(d.Pool), d.Tx, transitionApplier, d.InventoryRestore, d.Promotions, d.Logger,
		),
		RecoverStale: recoverstale.New(recoverstalepg.New(d.Pool), transitionApplier, d.Logger),
		Transition:   transitionApplier,
	}
}
