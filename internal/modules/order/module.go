package order

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/cancel"
	cancelpg "github.com/residwi/go-api-project-template/internal/modules/order/cancel/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/changestatus"
	changestatuspg "github.com/residwi/go-api-project-template/internal/modules/order/changestatus/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/expire"
	expirepg "github.com/residwi/go-api-project-template/internal/modules/order/expire/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/place"
	placepg "github.com/residwi/go-api-project-template/internal/modules/order/place/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/order/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/recoverstale"
	recoverstalepg "github.com/residwi/go-api-project-template/internal/modules/order/recoverstale/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/retrypayment"
	retrypaymentpg "github.com/residwi/go-api-project-template/internal/modules/order/retrypayment/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order/transition"
	transitionpg "github.com/residwi/go-api-project-template/internal/modules/order/transition/postgres"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Tx     database.TxRunner
	Logger *slog.Logger

	Cart          CartProvider
	Inventory     InventoryPort
	Promotions    CouponPort
	Notifications NotificationEnqueuer

	Payment     PaymentInitiator
	PaymentJobs PaymentJobCanceller
}

type CartProvider interface {
	LockCart(ctx context.Context, userID uuid.UUID) error
	GetSnapshot(ctx context.Context, userID uuid.UUID) (*cartcontract.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

type InventoryPort interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponPort interface {
	Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}

type Module struct {
	Place        *place.Command
	Query        *query.Reader
	RetryPayment *retrypayment.Command
	Cancel       *cancel.Command
	ChangeStatus *changestatus.Command
	Expire       *expire.Command
	RecoverStale *recoverstale.Command
	Transition   *transition.Applier
}

func New(d Deps) *Module {
	transitionApplier := transition.New(transitionpg.New(d.Pool))

	return &Module{
		Place: place.New(
			placepg.New(d.Pool), d.Tx, d.Cart, d.Inventory, d.Payment, d.Promotions, d.Notifications,
			transitionApplier, d.Logger,
		),
		Query:        query.New(querypg.New(d.Pool)),
		RetryPayment: retrypayment.New(retrypaymentpg.New(d.Pool), d.Payment),
		Cancel: cancel.New(
			cancelpg.New(d.Pool), d.Tx, transitionApplier, d.Inventory, d.Promotions, d.PaymentJobs, d.Logger,
		),
		ChangeStatus: changestatus.New(changestatuspg.New(d.Pool), transitionApplier),
		Expire: expire.New(
			expirepg.New(d.Pool), d.Tx, transitionApplier, d.Inventory, d.Promotions, d.Logger,
		),
		RecoverStale: recoverstale.New(recoverstalepg.New(d.Pool), transitionApplier, d.Logger),
		Transition:   transitionApplier,
	}
}

func (m *Module) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkPaymentProcessing(ctx, orderID)
}

func (m *Module) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkAwaitingPayment(ctx, orderID)
}

func (m *Module) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkPaid(ctx, orderID)
}

func (m *Module) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkFulfillmentFailedAfterCharge(ctx, orderID)
}

func (m *Module) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkFulfillmentFailedCompensating(ctx, orderID)
}

func (m *Module) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkRefunded(ctx, orderID)
}

func (m *Module) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkShipped(ctx, orderID)
}

func (m *Module) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return m.Transition.MarkDelivered(ctx, orderID)
}

func (m *Module) GetSnapshot(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetSnapshot(ctx, orderID)
}

func (m *Module) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetInfo(ctx, orderID)
}

func (m *Module) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	return m.Query.ListItemQuantities(ctx, orderID)
}

func (m *Module) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return m.Query.HasDeliveredOrder(ctx, userID, orderID, productID)
}

func (m *Module) ExpireStale(ctx context.Context) error {
	return m.Expire.Sweep(ctx)
}

func (m *Module) RecoverStaleProcessing(ctx context.Context) error {
	return m.RecoverStale.Sweep(ctx)
}
