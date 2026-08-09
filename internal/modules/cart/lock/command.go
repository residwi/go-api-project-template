package lock

import (
	"context"

	"github.com/google/uuid"
)

// Command has no route: order/place is its only caller, reaching it through
// order.CartProvider by name-match against cart.Module.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

// LockCart serializes concurrent checkouts of one cart. Returns
// apperror.ErrNotFound when the user has no cart.
//
// LockCart, not Execute: order.CartProvider names this method, and order/place
// calls it directly against cart.Module by name-match.
func (c *Command) LockCart(ctx context.Context, userID uuid.UUID) error {
	_, err := c.repo.GetCartForLock(ctx, userID)
	return err
}
