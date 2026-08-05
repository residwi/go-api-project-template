package payment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// OrderUpdater is intent methods, so payment never imports order: the bootstrap
// adapter maps each to the order.Transition that owns the allowed-from set.
type OrderUpdater interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
	// Returns a wrapped apperror.ErrBadRequest when the order is no longer
	// cancellable, e.g. already paid by a concurrent charge.
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
	Total      money.Money
	Status     string
	CouponCode string
	// Owned by the order module and persisted, not re-derived from Status. Payment
	// reads them to choose restock vs release, and to skip a reversal that already
	// happened rather than double-releasing.
	StockDeducted bool
	StockReversed bool
	// The order module owns the mapping from its status enum; payment reads the flag
	// to skip restocking on refund rather than re-deriving order semantics.
	Dispatched bool
}

type OrderGetter interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderSnapshot, error)
}

type InventoryChange struct {
	ProductID uuid.UUID
	Quantity  int
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items []InventoryChange) error
}

type InventoryRestorer interface {
	// Inventory owns the release-vs-restock choice; payment supplies only the
	// order's persisted fact, never the mechanics.
	Restore(ctx context.Context, items []InventoryChange, wasDeducted bool) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

// OrderHousekeeper is owned by the order module -- expiring stale orders,
// recovering ones stuck in payment_processing -- and supplied by bootstrap.
type OrderHousekeeper interface {
	ExpireStale(ctx context.Context) error
	RecoverStaleProcessing(ctx context.Context) error
}
