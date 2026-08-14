package adjust

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error) {
	return c.repo.AdjustStock(ctx, productID, newQuantity)
}
