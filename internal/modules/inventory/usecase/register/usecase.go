package register

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

func (c *UseCase) EnsureLevel(ctx context.Context, productID uuid.UUID) error {
	return c.repo.EnsureLevel(ctx, productID)
}
