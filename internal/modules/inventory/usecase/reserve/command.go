package reserve

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

func (c *Command) ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return c.repo.ReserveBatch(ctx, items)
}

func (c *Command) Reserve(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error) {
	return c.repo.Reserve(ctx, productID, qty)
}
