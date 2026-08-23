package review

import (
	"context"

	"github.com/google/uuid"
)

// PurchaseVerifier is satisfied by order.Service. It lives here, not on
// Create alone, because service.go is where a port fed by only one method
// still gets declared.
type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error)
}
