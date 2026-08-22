package dashboard

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Deps struct {
	Repo Repository
}

type Service struct {
	repo Repository
}

func New(d Deps) *Service {
	return &Service{repo: d.Repo}
}

func (s *Service) ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
	return s.repo.ListRevenueByDay(ctx, from, to)
}

func (s *Service) GetSummary(
	ctx context.Context,
	from, to time.Time,
) (domain.SalesSummary, []domain.StatusBreakdown, error) {
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

func (s *Service) ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error) {
	return s.repo.ListTopProducts(ctx, limit, from, to)
}
