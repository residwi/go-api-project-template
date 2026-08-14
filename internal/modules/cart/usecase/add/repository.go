package add

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	CountAndHasItem(ctx context.Context, cartID, productID uuid.UUID) (count int, hasProduct bool, err error)
	AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error
}
