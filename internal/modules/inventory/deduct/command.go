package deduct

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Command keeps the name DeductBatch, matching what payment.InventoryDeductor
// and order both declare, so they wire to it by name-match with no adapter.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) DeductBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return c.repo.DeductBatch(ctx, items)
}

// Deduct has no production caller -- payment/charge and order deduct a whole
// order at once, through DeductBatch. Kept rather than dropped: deleting it
// inside a refactor would hide it, and a slice holding an unused method is
// visible.
func (c *Command) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Deduct(ctx, productID, qty)
}
