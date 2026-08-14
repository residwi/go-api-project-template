package deduct

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) DeductBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return c.repo.DeductBatch(ctx, items)
}

func (c *UseCase) Deduct(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Deduct(ctx, productID, qty)
}
