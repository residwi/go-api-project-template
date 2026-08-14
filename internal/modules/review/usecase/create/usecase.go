package create

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
)

type Params struct {
	OrderID uuid.UUID
	Rating  int
	Title   string
	Body    string
}

type UseCase struct {
	repo     Repository
	purchase PurchaseVerifier
}

func New(repo Repository, purchase PurchaseVerifier) *UseCase {
	return &UseCase{repo: repo, purchase: purchase}
}

func (c *UseCase) Execute(ctx context.Context, userID, productID uuid.UUID, p Params) (*domain.Review, error) {
	delivered, err := c.purchase.HasDeliveredOrder(ctx, userID, p.OrderID, productID)
	if err != nil {
		return nil, err
	}
	if !delivered {
		return nil, fmt.Errorf(
			"%w: order must be a delivered order of yours containing this product",
			apperror.ErrBadRequest,
		)
	}

	reviewed, err := c.repo.HasUserReviewed(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if reviewed {
		return nil, apperror.ErrConflict
	}

	rv := &domain.Review{
		UserID:    userID,
		ProductID: productID,
		OrderID:   p.OrderID,
		Rating:    p.Rating,
		Title:     p.Title,
		Body:      p.Body,
		Status:    domain.StatusPublished,
	}

	if err := c.repo.Create(ctx, rv); err != nil {
		return nil, err
	}

	return rv, nil
}
