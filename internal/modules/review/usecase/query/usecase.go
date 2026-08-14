package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) ListByProduct(
	ctx context.Context,
	productID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Review, error) {
	return r.repo.ListByProduct(ctx, productID, cursor)
}

func (r *UseCase) GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error) {
	return r.repo.GetStats(ctx, productID)
}
