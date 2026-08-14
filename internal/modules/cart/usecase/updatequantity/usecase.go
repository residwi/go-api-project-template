package updatequantity

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
)

type Params struct {
	Quantity int
}

type UseCase struct {
	repo     Repository
	products ProductLookup
}

func New(repo Repository, products ProductLookup) *UseCase {
	return &UseCase{repo: repo, products: products}
}

func (c *UseCase) Execute(ctx context.Context, userID, productID uuid.UUID, p Params) error {
	info, err := c.products.GetInfo(ctx, productID)
	if err != nil {
		return err
	}
	if info.Status != domain.StatusPublished {
		return fmt.Errorf("%w: product is not available", apperror.ErrBadRequest)
	}
	if info.Available < p.Quantity {
		return apperror.ErrInsufficientStock
	}

	cartID, err := c.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return c.repo.UpdateItemQuantity(ctx, cartID, productID, p.Quantity)
}
