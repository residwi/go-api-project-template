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

// PaymentJobCanceller is a constructor argument, not a setter: at slice
// granularity the order/payment cycle runs through four packages
// (order/transition, order/query, payment/charge, payment/jobs), not two, so
// bootstrap builds order's and payment's shared reads first, then payment,
// then hands payment.Jobs in here at construction time. Named
// CancelPendingByOrderID to match the capability name payment/jobs exports.
type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}
