package inventory

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/inventory/domain"
)

type Repository interface {
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Stock, error)
	Reserve(ctx context.Context, items map[uuid.UUID]int) error
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
	ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error
	RestockBatch(ctx context.Context, items map[uuid.UUID]int) error
}
