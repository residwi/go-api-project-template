package create

import (
	"context"

	"github.com/google/uuid"
)

type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error)
}
