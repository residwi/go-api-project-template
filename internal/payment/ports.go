package payment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

// OrderUpdater drives order-status changes from the payment domain via intent
// methods, so payment never imports the order package; the bootstrap adapter maps
// each method to the corresponding order.Transition (which owns the allowed-from
// set).
type OrderUpdater interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
	// CancelUnpaid cancels an order whose payment terminally failed and releases
	// its reserved stock and coupon. Returns a wrapped apperror.ErrBadRequest when the
	// order is no longer cancellable (e.g. already paid by a concurrent charge).
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

type OrderItemDTO struct {
	ProductID uuid.UUID
	Quantity  int
}

type OrderItemsGetter interface {
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]OrderItemDTO, error)
}

type OrderSnapshot struct {
	// Total is the order's charged amount. Denominated, so the finalization check
	// against the payment's own amount is one comparison rather than two fields
	// that a future edit could get out of step.
	Total      money.Money
	Status     string
	CouponCode string
	// StockDeducted reports whether the order's inventory was deducted from
	// stock (vs only reserved); StockReversed reports whether the hold was
	// already released/restocked. The order module owns these facts (persisted,
	// not re-derived from Status); payment uses them to choose restock vs release
	// and to skip a reversal that already happened (avoiding a double release).
	StockDeducted bool
	StockReversed bool
	// Dispatched reports whether the order's goods have physically left the
	// warehouse (shipped/delivered). The order module owns the mapping from its
	// status enum; payment reads this flag to skip restocking on refund rather
	// than re-deriving order semantics from a status string it can't import.
	Dispatched bool
}

type OrderGetter interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderSnapshot, error)
}

// InventoryChange is one product/quantity pair for a batched inventory op.
type InventoryChange struct {
	ProductID uuid.UUID
	Quantity  int
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items []InventoryChange) error
}

type InventoryRestorer interface {
	// Restore reverses an order's inventory effect; wasDeducted selects release
	// vs restock. Inventory owns that choice — payment only supplies the order's
	// fact (computed from its snapshot), not the mechanics.
	Restore(ctx context.Context, items []InventoryChange, wasDeducted bool) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

// OrderHousekeeper runs the order module's per-tick housekeeping: expiring stale
// awaiting_payment orders and recovering orders stuck in payment_processing
// (e.g. after a worker died mid-charge). It's an inline cross-feature interface
// (like OrderUpdater/OrderGetter); the order module owns the logic and the
// bootstrap adapter supplies it.
type OrderHousekeeper interface {
	ExpireStale(ctx context.Context) error
	RecoverStaleProcessing(ctx context.Context) error
}
