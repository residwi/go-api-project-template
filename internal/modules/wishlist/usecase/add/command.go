package add

import (
	"context"

	"github.com/google/uuid"
)

type Params struct {
	ProductID uuid.UUID
}

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
