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

func (r *Reader) ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
	return r.repo.ListRevenueByDay(ctx, from, to)
}
