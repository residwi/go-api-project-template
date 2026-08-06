package product

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
)

// InventoryReader answers for a whole page at once. A per-product lookup would
// turn every list endpoint into N+1 queries, which is the failure mode that
// pushes people back to a cross-module JOIN.
type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventorycontract.Availability, error)
}

// InventoryRegistrar takes no initial quantity: writing stock from product's
// create transaction would be a cross-module write.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
