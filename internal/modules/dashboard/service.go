package dashboard

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetSummary(ctx context.Context, from, to time.Time) (SalesSummary, []StatusBreakdown, error) {
	var (
		sales     SalesSummary
		breakdown []StatusBreakdown
	)

	// Pass gctx, not ctx: errgroup cancels it the moment either query fails, so the
	// sibling stops instead of running on against a doomed context.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		sales, err = s.repo.GetSalesSummary(gctx, from, to)
		return err
	})
	g.Go(func() error {
		var err error
		breakdown, err = s.repo.GetOrderStatusBreakdown(gctx, from, to)
		return err
	})
	if err := g.Wait(); err != nil {
		return SalesSummary{}, nil, err
	}
	return sales, breakdown, nil
}

func (s *Service) GetSalesSummary(ctx context.Context, from, to time.Time) (SalesSummary, error) {
	return s.repo.GetSalesSummary(ctx, from, to)
}

func (s *Service) GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]TopProduct, error) {
	return s.repo.GetTopProducts(ctx, limit, from, to)
}

func (s *Service) GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error) {
	return s.repo.GetRevenueByDay(ctx, from, to)
}

func (s *Service) GetOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]StatusBreakdown, error) {
	return s.repo.GetOrderStatusBreakdown(ctx, from, to)
}
