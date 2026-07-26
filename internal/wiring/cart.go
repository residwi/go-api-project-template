package wiring

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func NewCartService(repo cart.Repository, tx database.TxRunner, productSvc *product.Service, maxCartItems int) *cart.Service {
	return cart.NewService(repo, tx, &productLookupAdapter{svc: productSvc}, maxCartItems)
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
		Available: p.Availability.Available,
	}, nil
}

func (a *productLookupAdapter) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]cart.ProductInfo, error) {
	products, err := a.svc.GetByIDsIncludingDeleted(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]cart.ProductInfo, len(products))
	for _, p := range products {
		out[p.ID] = cart.ProductInfo{
			ID:        p.ID,
			Name:      p.Name,
			Price:     p.Price,
			Currency:  p.Currency,
			Status:    p.Status,
			Available: p.Availability.Available,
		}
	}
	return out, nil
}
