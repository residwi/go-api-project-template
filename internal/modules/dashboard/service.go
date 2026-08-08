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

func (s *Service) GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error) {
	return s.repo.GetRevenueByDay(ctx, from, to)
}
