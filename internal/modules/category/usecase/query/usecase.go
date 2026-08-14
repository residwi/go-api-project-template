package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) List(ctx context.Context) ([]domain.Category, error) {
	return r.repo.List(ctx)
}

func (r *UseCase) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	return r.repo.GetBySlug(ctx, slug)
}
