package remove

import (
	"context"

	"github.com/google/uuid"
)

// Repository is remove's own storage. Its only implementation is
// remove/postgres, constructed in wishlist/module.go.
type Repository interface {
	RemoveItem(ctx context.Context, userID, productID uuid.UUID) error
}
