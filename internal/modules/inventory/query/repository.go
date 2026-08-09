package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
	// GetLevels leaves a missing id absent from the map: the caller decides
	// what that means.
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Stock, error)
}
