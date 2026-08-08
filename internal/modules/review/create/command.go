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

// Command takes no TxRunner: the old service ran HasDeliveredOrder,
// HasUserReviewed and Create as three unwrapped calls with nothing atomic
// binding them, and this move preserves that rather than introducing
// atomicity that was never there.
type Command struct {
	repo     Repository
	purchase PurchaseVerifier
}

func New(repo Repository, purchase PurchaseVerifier) *Command {
	return &Command{repo: repo, purchase: purchase}
}

func (c *Command) Execute(ctx context.Context, userID, productID uuid.UUID, p Params) (*domain.Review, error) {
	// The specific client-supplied order, or p.OrderID could name any existing one
	// and forge the review's provenance.
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
