package lock

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

func (c *Command) LockCart(ctx context.Context, userID uuid.UUID) error {
	_, err := c.repo.GetCartForLock(ctx, userID)
	return err
}
