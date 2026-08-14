package restore

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) Restore(ctx context.Context, items map[uuid.UUID]int, prior contract.StockState) error {
	if prior == contract.Deducted {
		return c.repo.RestockBatch(ctx, items)
	}
	return c.repo.ReleaseBatch(ctx, items)
}

func (c *UseCase) Release(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Release(ctx, productID, qty)
}
