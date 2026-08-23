package shipping

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
)

// OrderGetter is satisfied by order.Service. Create and GetForUser both ask it
// for the same projection; shipping reads only Status and UserID off it, both
// of which Snapshot populates from the order row.
type OrderGetter interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
}

// OrderShipper and OrderDeliverer each feed exactly one method (Create,
// Deliver) but still live here, not beside either method, because service.go
// is where a port fed by only one method still gets declared.
type OrderShipper interface {
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
}

type OrderDeliverer interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
