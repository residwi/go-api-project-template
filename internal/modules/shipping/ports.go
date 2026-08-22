package shipping

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

// OrderGetter is satisfied by order's query use case. Create and GetForUser
// both ask it for GetInfo -- the old create and query slices declared the
// identical interface separately; the merge here is exact, not a judgment
// call.
type OrderGetter interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

// OrderShipper and OrderDeliverer each feed exactly one method (Create,
// Deliver) but still live here, not beside either method, because
// module.go -- now service.go -- is where a port fed by only one method
// still gets declared once the slice that used to own it is gone.
type OrderShipper interface {
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
}

type OrderDeliverer interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
