package create

import (
	"context"

	"github.com/google/uuid"
)

// InventoryRegistrar takes no initial quantity: writing stock from product's
// create transaction would be a cross-module write.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
