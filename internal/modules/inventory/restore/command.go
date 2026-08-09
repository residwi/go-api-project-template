package restore

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

// Restore keeps the release-vs-restock choice here, so callers need not know
// that a reservation and a deduction unwind differently.
func (c *Command) Restore(ctx context.Context, items map[uuid.UUID]int, prior contract.StockState) error {
	if prior == contract.Deducted {
		return c.repo.RestockBatch(ctx, items)
	}
	return c.repo.ReleaseBatch(ctx, items)
}

// Release has no production caller -- order/cancel and payment/refund restore
// a whole order at once, through Restore. Kept rather than dropped: deleting
// it inside a refactor would hide it, and a slice holding an unused method is
// visible.
func (c *Command) Release(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Release(ctx, productID, qty)
}
