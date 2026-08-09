package expire

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

// InventoryRestorer is narrower than order's old whole-service port: expire
// only ever releases a hold -- an awaiting_payment order's stock is always
// reserved, never deducted.
type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}
