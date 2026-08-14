package lock

import (
	"context"

	"github.com/google/uuid"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) LockCart(ctx context.Context, userID uuid.UUID) error {
	_, err := c.repo.GetCartForLock(ctx, userID)
	return err
}
