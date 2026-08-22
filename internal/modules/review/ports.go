package review

import (
	"context"

	"github.com/google/uuid"
)

// PurchaseVerifier is satisfied by order's query use case. It lives here,
// not on Create alone, because module.go -- now service.go -- is where a
// port fed by only one method still gets declared once the slice that used
// to own it is gone.
type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error)
}
