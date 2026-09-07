package dashboard

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/residwi/go-api-project-template/internal/features/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo   Repository
	tracer trace.Tracer
}

func New(repo Repository) *Service {
	return &Service{
		repo:   repo,
		tracer: otel.Tracer("github.com/residwi/go-api-project-template/internal/features/dashboard"),
	}
}

func (s *Service) ListRevenueByDay(ctx context.Context, from, to time.Time) (_ []domain.RevenueData, err error) {
	ctx, span := s.tracer.Start(ctx, "dashboard.ListRevenueByDay")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListRevenueByDay(ctx, from, to)
}

func (s *Service) GetSummary(
	ctx context.Context,
	from, to time.Time,
) (_ domain.SalesSummary, _ []domain.StatusBreakdown, err error) {
	ctx, span := s.tracer.Start(ctx, "dashboard.GetSummary")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	var (
		sales     domain.SalesSummary
		breakdown []domain.StatusBreakdown
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		sales, err = s.repo.GetSalesSummary(gctx, from, to)
		return err
	})
	g.Go(func() error {
		var err error
		breakdown, err = s.repo.ListOrderStatusBreakdown(gctx, from, to)
		return err
	})
	if err := g.Wait(); err != nil {
		return domain.SalesSummary{}, nil, err
	}
	return sales, breakdown, nil
}

func (s *Service) ListTopProducts(
	ctx context.Context,
	limit int,
	from, to time.Time,
) (_ []domain.TopProduct, err error) {
	ctx, span := s.tracer.Start(ctx, "dashboard.ListTopProducts")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.ListTopProducts(ctx, limit, from, to)
}
