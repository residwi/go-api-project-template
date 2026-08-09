package remove

import (
	"context"

	"github.com/google/uuid"
)

// Repository is remove's own storage. Its only implementation is
// remove/postgres, constructed in cart/module.go.
type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error
}
