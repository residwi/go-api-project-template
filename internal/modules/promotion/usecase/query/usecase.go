package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Params struct {
	paging.OffsetPage
}

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) ListAdmin(ctx context.Context, params Params) ([]domain.Promotion, int, error) {
	return r.repo.ListAdmin(ctx, params)
}
