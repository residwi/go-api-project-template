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

// GetStats has no caller yet -- no route and no other module reaches it -- but
// it carries a real aggregate query with its own repository test, so it moves
// here intact rather than being dropped like a bare pass-through would be.
func (r *Reader) GetStats(ctx context.Context, productID uuid.UUID) (domain.Stats, error) {
	return r.repo.GetStats(ctx, productID)
}
