package inventory

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is the deduplicated union of all seven slices' storage ports.
// Nothing collided: every slice owned a distinct query, so this list is a
// straight merge, not a dedup.
type Repository interface {
	AdjustStock(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
	GetLevels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Stock, error)

	// Reserve and Deduct lost their "Batch" suffix along with the singular
	// per-product methods that used to justify it -- deleting the dead
	// Reserve(ctx, productID, qty) and Deduct(ctx, productID, qty) freed the
	// unsuffixed names.
	Reserve(ctx context.Context, items map[uuid.UUID]int) error
	Deduct(ctx context.Context, items map[uuid.UUID]int) error

	// ReleaseBatch and RestockBatch keep their names: Restore dispatches to
	// one or the other depending on StockState, so the two queries stay
	// distinct even though the singular Release they used to sit beside is
	// gone.
	ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error
	RestockBatch(ctx context.Context, items map[uuid.UUID]int) error
}
