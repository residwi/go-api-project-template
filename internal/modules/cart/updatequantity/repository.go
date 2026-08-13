package updatequantity

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error
}
