package topproducts

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

func (r *UseCase) ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error) {
	return r.repo.ListTopProducts(ctx, limit, from, to)
}
