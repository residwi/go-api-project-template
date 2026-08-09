package cancel

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// TransitionApplier reaches transition/ through this narrow port instead of
// importing it.
type TransitionApplier interface {
	Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error
}

// InventoryRestorer is narrower than order's old whole-service port: cancel
// only ever restores a hold, it never reserves or deducts one.
type InventoryRestorer interface {
	// Inventory owns the release-vs-restock choice; cancel supplies only the
	// order's persisted fact, never the mechanics.
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

// PaymentJobCanceller is set after construction by SetPaymentDeps: payment is
// not sliced yet, and order/payment need each other at construction time, so
// bootstrap wires this one after both exist.
type PaymentJobCanceller interface {
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
}
