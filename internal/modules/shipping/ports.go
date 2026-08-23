package shipping

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
)

type OrderGetter interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
}

type OrderShipper interface {
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
}

type OrderDeliverer interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
