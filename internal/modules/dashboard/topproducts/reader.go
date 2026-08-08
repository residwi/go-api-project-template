package topproducts

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

func (r *Reader) GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error) {
	return r.repo.GetTopProducts(ctx, limit, from, to)
}
