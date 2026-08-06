package review

import (
	"context"

	"github.com/google/uuid"
)

// PurchaseVerifier takes three uuid.UUID arguments in a fixed order: userID,
// orderID, productID. The struct that used to name them is gone, so keep the
// order right at every call.
type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error)
}
