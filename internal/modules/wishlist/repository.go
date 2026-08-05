package wishlist

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	AddItem(ctx context.Context, wishlistID, productID uuid.UUID) error
	RemoveItem(ctx context.Context, userID, productID uuid.UUID) error
	GetItems(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]Item, error)
	HasItem(ctx context.Context, wishlistID, productID uuid.UUID) (bool, error)
}
