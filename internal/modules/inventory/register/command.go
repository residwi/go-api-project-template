package register

import (
	"context"

	"github.com/google/uuid"
)

// Command keeps the name EnsureLevel, matching what product.InventoryRegistrar
// declares, so product wires to it by name-match with no adapter. The package
// is named for the capability, not the method -- register is who, EnsureLevel
// is what it does.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

// EnsureLevel registers at zero stock: the initial quantity is set afterwards
// through inventory's own admin endpoint, not smuggled in on the product
// payload.
func (c *Command) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	return c.repo.EnsureLevel(ctx, productID)
}
