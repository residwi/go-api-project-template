package dashboard

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

func (s *Service) GetOrderStatusBreakdown(ctx context.Context) ([]StatusBreakdown, error) {
	return s.repo.GetOrderStatusBreakdown(ctx)
}
