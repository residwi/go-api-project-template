package product

import (
	"context"

	"github.com/google/uuid"
)

// Availability carries no reservation count: that is live order velocity per SKU,
// and product's responses are public.
type Availability struct {
	OnHand    int
	Available int
}

// InventoryReader answers for a whole page at once. A per-product lookup would
// turn every list endpoint into N+1 queries, which is the failure mode that
// pushes people back to a cross-module JOIN.
type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Availability, error)
}

// InventoryRegistrar takes no initial quantity: writing stock from product's
// create transaction would be a cross-module write.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
