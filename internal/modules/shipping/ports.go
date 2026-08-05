package shipping

import (
	"context"

	"github.com/google/uuid"
)

type OrderProvider interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderInfo, error)
}

type OrderInfo struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Status string
}

// OrderUpdater flips the order status from the shipping domain via intent
// methods; the bootstrap adapter maps each to the matching order.Transition.
type OrderUpdater interface {
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}
