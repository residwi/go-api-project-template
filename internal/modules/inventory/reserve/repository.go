package reserve

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
