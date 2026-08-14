package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) ListByProduct(
	ctx context.Context,
	productID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Review, error) {
	return r.repo.ListByProduct(ctx, productID, cursor)
}

func (r *Reader) GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error) {
	return r.repo.GetStats(ctx, productID)
}
