package empty

import (
	"context"

	"github.com/google/uuid"
)

// Command opens no transaction: it deletes through its own repository, with
// nothing else to coordinate.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

// Clear empties a cart. Named Clear, not Execute: order.CartProvider names
// this method, and cart.Module's own Clear delegates to it by the same name
// -- matching how query.Reader's GetSnapshot and lock.Command's LockCart are
// each named for the same reason.
func (c *Command) Clear(ctx context.Context, userID uuid.UUID) error {
	return c.repo.Clear(ctx, userID)
}
