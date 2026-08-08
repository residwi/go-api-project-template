package revenue

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) GetRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
	return r.repo.GetRevenueByDay(ctx, from, to)
}
