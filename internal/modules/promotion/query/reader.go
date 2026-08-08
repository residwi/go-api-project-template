package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Params struct {
	paging.OffsetPage
}

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) ListAdmin(ctx context.Context, params Params) ([]domain.Promotion, int, error) {
	return r.repo.ListAdmin(ctx, params)
}
