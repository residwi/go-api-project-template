package review

import (
	"context"

	"github.com/google/uuid"
)

// DeliveredPurchase names its fields because all three ids are uuid.UUID: a
// positional swap would compile and answer about the wrong purchase.
type DeliveredPurchase struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchase) (bool, error)
}
