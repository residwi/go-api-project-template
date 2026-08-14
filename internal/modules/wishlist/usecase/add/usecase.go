package add

import (
	"context"

	"github.com/google/uuid"
)

type Params struct {
	ProductID uuid.UUID
}

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) Execute(ctx context.Context, userID uuid.UUID, p Params) error {
	wishlistID, err := c.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return c.repo.AddItem(ctx, wishlistID, p.ProductID)
}
