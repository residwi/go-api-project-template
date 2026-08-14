package remove

import (
	"context"

	"github.com/google/uuid"
)

type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, userID, productID uuid.UUID) error {
	cartID, err := c.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return c.repo.RemoveItem(ctx, cartID, productID)
}
