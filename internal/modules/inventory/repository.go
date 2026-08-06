package inventory

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	RestockBatch(ctx context.Context, items map[uuid.UUID]int) error
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error)
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error)
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Stock, error)
}
