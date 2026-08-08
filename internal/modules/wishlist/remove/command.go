package remove

import (
	"context"

	"github.com/google/uuid"
)

// Command takes no TxRunner: it deletes one row through its own repository
// and asks nothing else (ARCHITECTURE.md decision 14).
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, userID, productID uuid.UUID) error {
	return c.repo.RemoveItem(ctx, userID, productID)
}
