package cart

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	CountAndHasItem(ctx context.Context, cartID, productID uuid.UUID) (count int, hasProduct bool, err error)
	AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error
	UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	GetCart(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
	GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
