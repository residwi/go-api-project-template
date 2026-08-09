package restock

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is restock's own storage. Its only implementation is
// restock/postgres, constructed in inventory/module.go.
type Repository interface {
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
