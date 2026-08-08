package add

import (
	"context"

	"github.com/google/uuid"
)

// Repository is add's own storage. Its only implementation is add/postgres,
// constructed in wishlist/module.go.
type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error
}
