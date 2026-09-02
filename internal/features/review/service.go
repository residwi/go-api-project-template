package review

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Service struct {
	repo     Repository
	purchase PurchaseVerifier
}

func New(repo Repository, purchase PurchaseVerifier) *Service {
	return &Service{repo: repo, purchase: purchase}
}

func (s *Service) Create(
	ctx context.Context,
	userID, productID, orderID uuid.UUID,
	rating int,
	title, body string,
) (*domain.Review, error) {
	delivered, err := s.purchase.HasDeliveredOrder(ctx, userID, orderID, productID)
	if err != nil {
		return nil, err
	}
	if !delivered {
		return nil, fmt.Errorf(
			"%w: order must be a delivered order of yours containing this product",
			errs.ErrBadRequest,
		)
	}

	reviewed, err := s.repo.HasUserReviewed(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if reviewed {
		return nil, errs.ErrConflict
	}

	rv := &domain.Review{
		UserID:    userID,
		ProductID: productID,
		OrderID:   orderID,
		Rating:    rating,
		Title:     title,
		Body:      body,
		Status:    domain.StatusPublished,
	}

	if err := s.repo.Create(ctx, rv); err != nil {
		return nil, err
	}

	return rv, nil
}

func (s *Service) ListByProduct(
	ctx context.Context,
	productID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Review, error) {
	return s.repo.ListByProduct(ctx, productID, cursor)
}

func (s *Service) GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error) {
	return s.repo.GetStats(ctx, productID)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
