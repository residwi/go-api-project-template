package cart

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error)
	AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, qty int) error
	RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error
	Clear(ctx context.Context, userID uuid.UUID) error
	CountItems(ctx context.Context, cartID uuid.UUID) (int, error)
	CountAndHasItem(ctx context.Context, cartID, productID uuid.UUID) (count int, hasProduct bool, err error)
	GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
