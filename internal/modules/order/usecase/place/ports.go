package place

import (
	"context"

	"github.com/google/uuid"

	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

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

type CouponReserver interface {
	Reserve(
		ctx context.Context,
		code string,
		userID uuid.UUID,
		orderID uuid.UUID,
		orderSubtotal int64,
	) (discountAmount int64, err error)
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

type TransitionApplier interface {
	Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error
}
