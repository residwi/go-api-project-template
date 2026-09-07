package wishlist

import (
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo   Repository
	tracer trace.Tracer
}

func New(repo Repository) *Service {
	return &Service{
		repo:   repo,
		tracer: otel.Tracer("github.com/residwi/go-api-project-template/internal/features/wishlist"),
	}
}

func (s *Service) Add(ctx context.Context, userID, productID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "wishlist.Add")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	wishlistID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.AddItem(ctx, wishlistID, productID)
}

func (s *Service) Remove(ctx context.Context, userID, productID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "wishlist.Remove")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.RemoveItem(ctx, userID, productID)
}

func (s *Service) List(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) (_ []domain.Item, err error) {
	ctx, span := s.tracer.Start(ctx, "wishlist.List")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListItemsForUser(ctx, userID, cursor)
}
