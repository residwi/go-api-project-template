package remove

import (
	"context"

	"github.com/google/uuid"
)

// Command takes no TxRunner: it deletes one row through its own repository and
// asks nothing else.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID) error {
	return c.repo.Delete(ctx, id)
}
