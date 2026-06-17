package dashboard

import (
	"context"
	"sync"
	"time"
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
		salesErr  error
		breakErr  error
	)

	const aggregateQueries = 2
	var wg sync.WaitGroup
	wg.Add(aggregateQueries)
	go func() {
		defer wg.Done()
		sales, salesErr = s.repo.GetSalesSummary(ctx, from, to)
	}()
	go func() {
		defer wg.Done()
		breakdown, breakErr = s.repo.GetOrderStatusBreakdown(ctx, from, to)
	}()
	wg.Wait()

	if salesErr != nil {
		return SalesSummary{}, nil, salesErr
	}
	if breakErr != nil {
		return SalesSummary{}, nil, breakErr
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
