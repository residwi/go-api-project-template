package revenue

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
	return r.repo.ListRevenueByDay(ctx, from, to)
}
