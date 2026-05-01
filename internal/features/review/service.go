package review

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
)

type PurchaseVerifier interface {
	HasDeliveredOrder(ctx context.Context, userID, productID uuid.UUID) (bool, error)
}

type Service struct {
	repo     Repository
	purchase PurchaseVerifier
}

func NewService(repo Repository, purchase PurchaseVerifier) *Service {
	return &Service{
		repo:     repo,
		purchase: purchase,
	}
}

func (s *Service) Create(ctx context.Context, userID, productID uuid.UUID, req CreateReviewRequest) (*Review, error) {
	delivered, err := s.purchase.HasDeliveredOrder(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if !delivered {
		return nil, fmt.Errorf("%w: must have a delivered order for this product", core.ErrBadRequest)
	}

	reviewed, err := s.repo.HasUserReviewed(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if reviewed {
		return nil, core.ErrConflict
	}

	rv := &Review{
		UserID:    userID,
		ProductID: productID,
		OrderID:   req.OrderID,
		Rating:    req.Rating,
		Title:     req.Title,
		Body:      req.Body,
		Status:    StatusPublished,
	}

	if err := s.repo.Create(ctx, rv); err != nil {
		return nil, err
	}

	return rv, nil
}

func (s *Service) ListByProduct(ctx context.Context, productID uuid.UUID, cursor core.CursorPage) ([]Review, error) {
	return s.repo.ListByProduct(ctx, productID, cursor)
}

func (s *Service) GetStats(ctx context.Context, productID uuid.UUID) (Stats, error) {
	return s.repo.GetStats(ctx, productID)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
