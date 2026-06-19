package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/product"
)

func NewCartService(repo cart.Repository, pool *pgxpool.Pool, productSvc *product.Service, maxCartItems int) *cart.Service {
	return cart.NewService(repo, pool, &productLookupAdapter{svc: productSvc}, maxCartItems)
}

type productLookupAdapter struct{ svc *product.Service }

func (a *productLookupAdapter) GetByID(ctx context.Context, id uuid.UUID) (*cart.ProductInfo, error) {
	p, err := a.svc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &cart.ProductInfo{
		ID:        p.ID,
		Name:      p.Name,
		Price:     p.Price,
		Currency:  p.Currency,
		Status:    p.Status,
		Available: p.StockQuantity - p.ReservedQuantity,
	}, nil
}
