package images

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
)

// InventoryReader backs AvailableQuantity, which has no production caller --
// see command.go. Kept as its own port rather than reusing query's, because a
// slice may not import a sibling's port.
type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventorycontract.Availability, error)
}
