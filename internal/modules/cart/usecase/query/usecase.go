package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
)

type UseCase struct {
	repo     Repository
	products ProductLookup
}

func New(repo Repository, products ProductLookup) *UseCase {
	return &UseCase{repo: repo, products: products}
}

func (r *UseCase) GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error) {
	c, err := r.repo.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(c.Items) == 0 {
		return c, nil
	}

	ids := make([]uuid.UUID, len(c.Items))
	for i := range c.Items {
		ids[i] = c.Items[i].ProductID
	}
	infos, err := r.products.GetInfoByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("looking up cart products: %w", err)
	}

	for i := range c.Items {
		info, ok := infos[c.Items[i].ProductID]
		if !ok {
			c.Items[i].Product = &domain.Product{Status: "unavailable"}
			continue
		}
		c.Items[i].Product = &domain.Product{
			Name:   info.Name,
			Price:  info.Price,
			Stock:  info.Available,
			Status: info.Status,
		}
	}
	return c, nil
}

func (r *UseCase) GetSnapshot(ctx context.Context, userID uuid.UUID) (*contract.Cart, error) {
	c, err := r.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	snap := &contract.Cart{ID: c.ID}
	for _, item := range c.Items {
		si := contract.CartItem{ProductID: item.ProductID, Quantity: item.Quantity}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}
