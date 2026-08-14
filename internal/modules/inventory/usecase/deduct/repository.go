package deduct

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Repository interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
