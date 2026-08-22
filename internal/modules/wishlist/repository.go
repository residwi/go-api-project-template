package wishlist

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error
	RemoveItem(ctx context.Context, userID, productID uuid.UUID) error
	ListItemsForUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Item, error)
}
