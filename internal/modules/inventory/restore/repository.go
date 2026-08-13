package restore

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error
	RestockBatch(ctx context.Context, items map[uuid.UUID]int) error
	Release(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
