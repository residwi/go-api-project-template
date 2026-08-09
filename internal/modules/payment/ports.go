package payment

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
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

type OrderItemsGetter interface {
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type OrderGetter interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryRestorer interface {
	// Inventory owns the release-vs-restock choice; payment supplies only the
	// order's persisted fact, never the mechanics.
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}
