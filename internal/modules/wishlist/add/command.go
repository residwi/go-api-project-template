package add

import (
	"context"

	"github.com/google/uuid"
)

type Params struct {
	ProductID uuid.UUID
}

// Command takes no TxRunner: the old service ran GetOrCreate and AddItem as
// two separate calls with nothing wrapping them, and this move keeps that
// behaviour rather than introducing atomicity that was never there.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, userID uuid.UUID, p Params) error {
	wishlistID, err := c.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return c.repo.AddItem(ctx, wishlistID, p.ProductID)
}
