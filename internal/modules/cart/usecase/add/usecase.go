package add

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Params struct {
	ProductID uuid.UUID
	Quantity  int
}

type UseCase struct {
	repo         Repository
	tx           database.TxRunner
	products     ProductLookup
	maxCartItems int
}

func New(repo Repository, tx database.TxRunner, products ProductLookup, maxCartItems int) *UseCase {
	return &UseCase{
		repo:         repo,
		tx:           tx,
		products:     products,
		maxCartItems: maxCartItems,
	}
}

func (c *UseCase) Execute(ctx context.Context, userID uuid.UUID, p Params) error {
	info, err := c.products.GetInfo(ctx, p.ProductID)
	if err != nil {
		return err
	}

	if info.Status != domain.StatusPublished {
		return fmt.Errorf("%w: product is not available", apperror.ErrBadRequest)
	}
	if info.Available < p.Quantity {
		return apperror.ErrInsufficientStock
	}

	return c.tx.Run(ctx, func(txCtx context.Context) error {
		cartID, err := c.repo.GetOrCreate(txCtx, userID)
		if err != nil {
			return err
		}

		count, hasItem, err := c.repo.CountAndHasItem(txCtx, cartID, p.ProductID)
		if err != nil {
			return err
		}
		if !hasItem && count >= c.maxCartItems {
			return fmt.Errorf("%w: cart cannot have more than %d items", apperror.ErrBadRequest, c.maxCartItems)
		}

		return c.repo.AddItem(txCtx, cartID, p.ProductID, p.Quantity)
	})
}
