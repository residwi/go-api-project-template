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

	// Payment and PaymentJobs are constructor arguments, not a setter: at
	// slice granularity the order/payment cycle runs through four packages
	// (order/transition, order/query, payment/charge, payment/jobs), not two,
	// so bootstrap builds order's own transition and query reads, then
	// payment (which needs them), then hands payment.Charge and payment.Jobs
	// in here to finish building order.
	Payment     PaymentInitiator
	PaymentJobs PaymentJobCanceller
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

// PaymentInitiator and PaymentJobCanceller are constructor arguments: at
// slice granularity the order/payment cycle runs through four packages, not
// two, so bootstrap can build payment.Charge and payment.Jobs before order
// needs them, and hand them in here instead of setting them after the fact.
type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}

// Module is Place, Query, RetryPayment, Cancel, ChangeStatus, Expire,
// RecoverStale and Transition. shipping's OrderPort, review's
// PurchaseVerifier and cmd/worker's OrderHousekeeper each ask for a subset of
// this module's capabilities as one whole-module value (bootstrap passes
// ordMod itself for each), which is why Module still exposes the
// MarkShipped/MarkDelivered/GetInfo/HasDeliveredOrder/ExpireStale/
// RecoverStaleProcessing delegators below: a single Go value can only satisfy
// those interfaces by carrying the methods itself.
//
// payment no longer consumes ordMod this way: at slice granularity it reads
// order/transition and order/query directly, built early enough in
// bootstrap.New that no whole-module value is needed. The remaining Mark*,
// GetSnapshot and ListItemQuantities delegators below now have no caller
// outside this package's own tests -- left in place rather than pruned in
// this task, since removing them is an order.Module API change this task's
// mandate did not ask for.
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

// The eight Mark* delegators below are passthroughs to Transition, which is
// where every allowed-from set actually lives. MarkShipped and MarkDelivered
// back shipping's OrderPort; the other six had no caller left once payment
// started reading order/transition directly instead of through this Module.

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

// GetSnapshot had no caller left once payment started reading order/query
// directly instead of through this Module.
func (m *Module) GetSnapshot(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetSnapshot(ctx, orderID)
}

// GetInfo backs shipping's OrderPort.
func (m *Module) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	return m.Query.GetInfo(ctx, orderID)
}

// ListItemQuantities had no caller left once payment started reading
// order/query directly instead of through this Module.
func (m *Module) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	return m.Query.ListItemQuantities(ctx, orderID)
}

// HasDeliveredOrder backs review's PurchaseVerifier.
func (m *Module) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return m.Query.HasDeliveredOrder(ctx, userID, orderID, productID)
}

// ExpireStale and RecoverStaleProcessing back cmd/worker's
// paymentworker.OrderHousekeeper, which wires them into payment's own queue
// runner as its per-tick Sweep hook -- the same shape as before order was
// sliced. They also keep test/e2e's existing call surface
// (order_expiry_test.go) intact.

func (m *Module) ExpireStale(ctx context.Context) error {
	return m.Expire.Sweep(ctx)
}

func (m *Module) RecoverStaleProcessing(ctx context.Context) error {
	return m.RecoverStale.Sweep(ctx)
}
