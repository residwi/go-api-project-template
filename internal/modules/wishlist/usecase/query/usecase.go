package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) ListItemsForUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Item, error) {
	return r.repo.ListItemsForUser(ctx, userID, cursor)
}
