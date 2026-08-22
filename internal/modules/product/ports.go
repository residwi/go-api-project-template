package product

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
)

// InventoryRegistrar is satisfied by inventory's register use case. It lives
// here, not beside Create alone, because module.go -- now service.go -- is
// where a port fed by only one method still gets declared once the slice
// that used to own it is gone.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}

// InventoryReader is satisfied by inventory's query use case. It stays a
// separate port from InventoryRegistrar because inventory has not
// flattened yet -- the two are still two different slice values, not one
// value with both methods.
type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventorycontract.Availability, error)
}
