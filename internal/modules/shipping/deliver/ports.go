package deliver

import (
	"context"

	"github.com/google/uuid"
)

// OrderPort is satisfied directly by order.Service.MarkDelivered -- no adapter.
// One method, because that is all this slice needs.
type OrderPort interface {
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
