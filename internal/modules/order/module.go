// Package order composes order's slices. It imports no transport package, so
// a worker or a future grpc server can construct this module without linking
// HTTP.
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

	// Cross-module ports, satisfied by name-match.
	Cart          CartProvider
	Inventory     InventoryPort
	Promotions    CouponPort
	Notifications NotificationEnqueuer
}

// CartProvider is what place needs from cart. cart.Module satisfies it
// directly by name-match.
type CartProvider interface {
	LockCart(ctx context.Context, userID uuid.UUID) error
	GetSnapshot(ctx context.Context, userID uuid.UUID) (*cartcontract.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// InventoryPort is the union of what place, cancel and expire need from
// inventory. inventory.Module satisfies it directly by name-match -- it holds
// the identical bundle for the same reason (order and payment are not sliced
// yet).
type InventoryPort interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

// CouponPort is what place (Reserve) and cancel/expire (Release) need from
// promotion. promotion.Module.Reserve satisfies it directly by name-match.
type CouponPort interface {
	Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

// PaymentInitiator and PaymentJobCanceller are set after construction by
// SetPaymentDeps: payment is not sliced yet, and order/payment need each
// other at construction time, so bootstrap wires these back in after both
// exist.
type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}

type PaymentJobCanceller interface {
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
}

// Module is Place, Query, RetryPayment, Cancel, ChangeStatus, Expire,
// RecoverStale and Transition -- payment is not sliced yet (task 13), and its
// OrderUpdater/OrderGetter/OrderItemsGetter/OrderHousekeeper ports, plus
// shipping's OrderPort and review's PurchaseVerifier, each ask for a subset of
// this module's capabilities as one whole-module value. That bundle is why
// Module itself exposes the Mark*/GetSnapshot/GetInfo/ListItemQuantities/
// HasDeliveredOrder/ExpireStale/RecoverStaleProcessing delegators below: a
// single Go value can only satisfy those interfaces by carrying the methods
// itself.
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
			placepg.New(d.Pool), d.Tx, d.Cart, d.Inventory, d.Promotions, d.Notifications, transitionApplier, d.Logger,
		),
		Query:        query.New(querypg.New(d.Pool)),
		RetryPayment: retrypayment.New(retrypaymentpg.New(d.Pool)),
		Cancel: cancel.New(
			cancelpg.New(d.Pool), d.Tx, transitionApplier, d.Inventory, d.Promotions, d.Logger,
		),
		ChangeStatus: changestatus.New(changestatuspg.New(d.Pool), transitionApplier),
		Expire: expire.New(
			expirepg.New(d.Pool), d.Tx, transitionApplier, d.Inventory, d.Promotions, d.Logger,
		),
		RecoverStale: recoverstale.New(recoverstalepg.New(d.Pool), transitionApplier, d.Logger),
		Transition:   transitionApplier,
	}
}

// SetPaymentDeps breaks the order/payment construction cycle: at whole-module
// granularity each needs the other, so bootstrap wires this one after both
// exist. Both ports are satisfied by payment.Service directly.
func (m *Module) SetPaymentDeps(payment PaymentInitiator, paymentCancel PaymentJobCanceller) {
	m.Place.SetPaymentDeps(payment)
	m.RetryPayment.SetPaymentDeps(payment)
	m.Cancel.SetPaymentDeps(paymentCancel)
}

// The eight Mark* delegators below are payment's OrderUpdater and shipping's
// OrderPort, satisfied without an adapter: each is a passthrough to
// Transition, which is where every allowed-from set actually lives.

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

// CancelUnpaid is payment's OrderUpdater intent method for the webhook's
// cancel-on-non-payment path, satisfied without an adapter.
func (m *Module) CancelUnpaid(ctx context.Context, orderID uuid.UUID) error {
	return m.Cancel.CancelUnpaid(ctx, orderID)
}

// GetSnapshot backs payment's OrderGetter.
func (m *Module) GetSnapshot(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetSnapshot(ctx, orderID)
}

// GetInfo backs shipping's OrderPort.
func (m *Module) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetInfo(ctx, orderID)
}

// ListItemQuantities backs payment's OrderItemsGetter.
func (m *Module) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	return m.Query.ListItemQuantities(ctx, orderID)
}

// HasDeliveredOrder backs review's PurchaseVerifier.
func (m *Module) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return m.Query.HasDeliveredOrder(ctx, userID, orderID, productID)
}

// ExpireStale and RecoverStaleProcessing back payment's OrderHousekeeper,
// which cmd/worker wires into payment's own queue runner as its per-tick
// Sweep hook -- the same shape as before order was sliced. They also keep
// test/e2e's existing call surface (order_expiry_test.go) intact.

func (m *Module) ExpireStale(ctx context.Context) error {
	return m.Expire.Sweep(ctx)
}

func (m *Module) RecoverStaleProcessing(ctx context.Context) error {
	return m.RecoverStale.Sweep(ctx)
}
