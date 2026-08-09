package restore

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is restore's own storage. Its only implementation is
// restore/postgres, constructed in inventory/module.go.
//
// ReleaseBatch and RestockBatch are the two undo primitives Command.Restore
// chooses between; nothing outside this package calls either directly, and
// Command exports no method of either name, so a caller cannot reach the
// mechanics even deliberately -- it supplies the prior contract.StockState and
// restore decides.
type Repository interface {
	ReleaseBatch(ctx context.Context, items map[uuid.UUID]int) error
	RestockBatch(ctx context.Context, items map[uuid.UUID]int) error
	// Release has no caller: order/cancel and payment/refund always restore a
	// whole order's items in one call, through Restore. Carried rather than
	// dropped -- see Command.Release.
	Release(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
