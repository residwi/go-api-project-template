package query

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
