package adjust

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
}
