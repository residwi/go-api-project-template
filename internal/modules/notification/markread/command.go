package markread

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

func (c *Command) Execute(ctx context.Context, userID, id uuid.UUID) error {
	return c.repo.MarkRead(ctx, userID, id)
}
