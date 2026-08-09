package reserve

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Command keeps the name ReserveBatch, matching what order.InventoryReserver
// declares, so order wires to it by name-match with no adapter.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return c.repo.ReserveBatch(ctx, items)
}

// Reserve has no production caller -- order/place reserves a whole cart at
// once, through ReserveBatch. Kept rather than dropped: deleting it inside a
// refactor would hide it, and a slice holding an unused method is visible.
func (c *Command) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Reserve(ctx, productID, qty)
}
