package product

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
)

type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}

type InventoryReader interface {
	GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]inventory.Availability, error)
}
