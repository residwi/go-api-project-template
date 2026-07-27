package inventory

import (
	"context"

	"github.com/google/uuid"
)

// StockChange is one product/quantity pair for a batched inventory operation.
type StockChange struct {
	ProductID uuid.UUID
	Quantity  int
}

// Repository is inventory's persistence port. The Postgres implementation
// lives in the postgres subpackage; this package never imports it.
type Repository interface {
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Release(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	ReserveBatch(ctx context.Context, items []StockChange) error
	ReleaseBatch(ctx context.Context, items []StockChange) error
	DeductBatch(ctx context.Context, items []StockChange) error
	RestockBatch(ctx context.Context, items []StockChange) error
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*Stock, error)
	GetStock(ctx context.Context, productID uuid.UUID) (*Stock, error)
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*Stock, error)
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Stock, error)
}
