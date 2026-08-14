package markallread

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

func (c *Command) Execute(ctx context.Context, userID uuid.UUID) error {
	return c.repo.MarkAllRead(ctx, userID)
}
