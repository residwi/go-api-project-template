package remove

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error
}
