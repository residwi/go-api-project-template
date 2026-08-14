package remove

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

func (c *UseCase) Execute(ctx context.Context, userID, productID uuid.UUID) error {
	return c.repo.RemoveItem(ctx, userID, productID)
}
