package product

import (
	"context"

	"github.com/google/uuid"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

// Availability is what product needs to know about stock. It deliberately does
// not expose the reservation count: that is live order velocity per SKU, and
// product's responses are public.
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

// InventoryRegistrar registers a product with inventory at zero stock. Product
// cannot set an initial quantity: stock is inventory's data, and writing it from
// product's create transaction is exactly the cross-module write this refactor
// removes.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
