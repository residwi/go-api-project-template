package adjust

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is adjust's own storage. Its only implementation is
// adjust/postgres, constructed in inventory/module.go.
type Repository interface {
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
}
