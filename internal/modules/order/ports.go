package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
)

type CartLocker interface {
	Lock(ctx context.Context, userID uuid.UUID) error
}

type CartReader interface {
	Snapshot(ctx context.Context, userID uuid.UUID) (*cart.Snapshot, error)
}

type CartClearer interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}

type InventoryReserver interface {
	Reserve(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryDeductor interface {
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventory.StockState) error
}

// CouponReserver is place's CouponReserver plus cancel's and expire's
// CouponReleaser. Reserve and Release are two halves of one coupon hold, so
// they stay one port rather than three; promotion's own service satisfies it
// by name-match. Nil is a supported value: an order placed with no coupon
// code never reaches either method.
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

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}
