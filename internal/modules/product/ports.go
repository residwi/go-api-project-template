package product

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
)

// InventoryRegistrar is satisfied by inventory.Service.EnsureLevel. It lives
// here, not beside Create alone, because module.go -- now service.go -- is
// where a port fed by only one method still gets declared once the slice
// that used to own it is gone.
type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}

// InventoryReader is satisfied by inventory.Service.GetAvailability. It
// stays a separate port from InventoryRegistrar even though both are
// satisfied by the same *inventory.Service value now -- product asks for
// two different capabilities, and the port boundary keeps each caller
// naming only the one it needs.
type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventory.Availability, error)
}
