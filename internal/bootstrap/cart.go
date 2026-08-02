package bootstrap

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/product"
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
		status := p.Status
		if p.DeletedAt != nil {
			// product.Delete only sets deleted_at -- it leaves status untouched --
			// so a withdrawn product's row can still read status='published'. Report
			// the soft delete honestly instead of forwarding that stale value: cart
			// (and order's availability guard downstream) must see this line as
			// unsellable, not silently drop it.
			status = "unavailable"
		}
		out[p.ID] = cart.ProductInfo{
			ID:        p.ID,
			Name:      p.Name,
			Price:     p.Price,
			Status:    status,
			Available: p.Availability.Available,
		}
	}
	return out, nil
}
