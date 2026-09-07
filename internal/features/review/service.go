package review

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo     Repository
	purchase PurchaseVerifier
	tracer   trace.Tracer
}

func New(repo Repository, purchase PurchaseVerifier) *Service {
	return &Service{
		repo:     repo,
		purchase: purchase,
		tracer:   otel.Tracer("github.com/residwi/go-api-project-template/internal/features/review"),
	}
}

func (s *Service) Create(
	ctx context.Context,
	userID, productID, orderID uuid.UUID,
	rating int,
	title, body string,
) (_ *domain.Review, err error) {
	ctx, span := s.tracer.Start(ctx, "review.Create")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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
) (_ []domain.Review, err error) {
	ctx, span := s.tracer.Start(ctx, "review.ListByProduct")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListByProduct(ctx, productID, cursor)
}

func (s *Service) GetStats(ctx context.Context, productID uuid.UUID) (_ domain.Stats, err error) {
	ctx, span := s.tracer.Start(ctx, "review.GetStats")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.GetStats(ctx, productID)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "review.Delete")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Delete(ctx, id)
}
