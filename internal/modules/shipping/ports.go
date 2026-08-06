package shipping

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

type OrderProvider interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

// OrderUpdater flips the order status from the shipping domain via intent
// methods; the bootstrap adapter maps each to the matching order.Transition.
type OrderUpdater interface {
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
