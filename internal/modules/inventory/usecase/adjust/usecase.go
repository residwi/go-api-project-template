package adjust

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

func (c *UseCase) Execute(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error) {
	return c.repo.AdjustStock(ctx, productID, newQuantity)
}
