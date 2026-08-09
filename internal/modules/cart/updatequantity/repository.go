package updatequantity

import (
	"context"

	"github.com/google/uuid"
)

// Repository is updatequantity's own storage. Its only implementation is
// updatequantity/postgres, constructed in cart/module.go.
type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error
}
