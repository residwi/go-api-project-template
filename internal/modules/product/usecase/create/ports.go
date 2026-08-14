package create

import (
	"context"

	"github.com/google/uuid"
)

type InventoryRegistrar interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
