package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
)

type Cart interface {
	Lock(ctx context.Context, userID uuid.UUID) error
	Snapshot(ctx context.Context, userID uuid.UUID) (*cart.Snapshot, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

type Inventory interface {
	Reserve(ctx context.Context, items map[uuid.UUID]int) error
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventory.StockState) error
}

type CouponReserver interface {
	Reserve(
		ctx context.Context,
		code string,
		userID uuid.UUID,
		orderID uuid.UUID,
		orderSubtotal int64,
	) (discountAmount int64, err error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type Notifications interface {
	Create(ctx context.Context, in notification.NewNotification) error
}
