package register

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

func (c *Command) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	return c.repo.EnsureLevel(ctx, productID)
}
