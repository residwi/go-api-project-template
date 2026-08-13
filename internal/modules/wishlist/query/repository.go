package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	ListItemsForUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Item, error)
}
