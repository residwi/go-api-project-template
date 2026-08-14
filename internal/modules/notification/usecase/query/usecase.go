package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Notification, error) {
	return r.repo.ListByUser(ctx, userID, cursor)
}

func (r *UseCase) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	return r.repo.CountUnread(ctx, userID)
}
