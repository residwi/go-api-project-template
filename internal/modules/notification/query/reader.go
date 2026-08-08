package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Notification, error) {
	return r.repo.ListByUser(ctx, userID, cursor)
}

func (r *Reader) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return r.repo.CountUnread(ctx, userID)
}
