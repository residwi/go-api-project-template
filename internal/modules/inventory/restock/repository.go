package restock

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
