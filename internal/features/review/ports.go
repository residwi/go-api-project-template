package review

import (
	"context"

	"github.com/google/uuid"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

// DeliveredPurchase names each id so a caller cannot transpose them; all three
// are uuid.UUID and a positional swap would compile and silently return the
// wrong verdict on whether this customer may review this product.
type DeliveredPurchase struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchase) (bool, error)
}
