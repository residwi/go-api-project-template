package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) List(ctx context.Context) ([]domain.Category, error) {
	return r.repo.List(ctx)
}

func (r *Reader) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	return r.repo.GetBySlug(ctx, slug)
}
