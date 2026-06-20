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

// GetSummary fetches the sales summary and order-status breakdown for the window.
// The two aggregates are independent, so they run concurrently (one round trip
// of wall-clock instead of the sum of both).
func (s *Service) GetSummary(ctx context.Context, from, to time.Time) (SalesSummary, []StatusBreakdown, error) {
	var (
		sales     SalesSummary
		breakdown []StatusBreakdown
	)

	// errgroup derives a context that is cancelled the moment either query
	// returns an error, so the sibling query is signalled to stop instead of
	// running to completion against a context (or pgx.Tx) that's already
	// doomed. Pass gctx — not the raw ctx — to the repo calls.
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
