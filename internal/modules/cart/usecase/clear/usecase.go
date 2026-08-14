package clear

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

func (c *UseCase) Clear(ctx context.Context, userID uuid.UUID) error {
	return c.repo.Clear(ctx, userID)
}
