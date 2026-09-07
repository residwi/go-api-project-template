package inventory

import (
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/features/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo   Repository
	tracer trace.Tracer
}

func New(repo Repository) *Service {
	return &Service{
		repo:   repo,
		tracer: otel.Tracer("github.com/residwi/go-api-project-template/internal/features/inventory"),
	}
}

func (s *Service) Adjust(ctx context.Context, productID uuid.UUID, newQuantity int) (_ *domain.Stock, err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.Adjust")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.AdjustStock(ctx, productID, newQuantity)
}

func (s *Service) Restock(ctx context.Context, productID uuid.UUID, qty int) (_ *domain.Stock, err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.Restock")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Restock(ctx, productID, qty)
}

func (s *Service) EnsureLevel(ctx context.Context, productID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.EnsureLevel")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.EnsureLevel(ctx, productID)
}

func (s *Service) GetStock(ctx context.Context, productID uuid.UUID) (_ *domain.Stock, err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.GetStock")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.GetStock(ctx, productID)
}

func (s *Service) GetAvailability(
	ctx context.Context,
	ids []uuid.UUID,
) (_ map[uuid.UUID]Availability, err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.GetAvailability")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	levels, err := s.repo.GetLevels(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]Availability, len(levels))
	for id, st := range levels {
		out[id] = Availability{OnHand: st.Quantity, Available: st.Available}
	}
	return out, nil
}

func (s *Service) Reserve(ctx context.Context, items map[uuid.UUID]int) (err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.Reserve")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Reserve(ctx, items)
}

func (s *Service) Deduct(ctx context.Context, items map[uuid.UUID]int) (err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.Deduct")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Deduct(ctx, items)
}

func (s *Service) Restore(ctx context.Context, items map[uuid.UUID]int, prior StockState) (err error) {
	ctx, span := s.tracer.Start(ctx, "inventory.Restore")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	switch prior {
	case StockDeducted:
		return s.repo.RestockBatch(ctx, items)
	case StockReserved:
	}
	return s.repo.ReleaseBatch(ctx, items)
}
