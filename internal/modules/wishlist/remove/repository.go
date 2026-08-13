package remove

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	RemoveItem(ctx context.Context, userID, productID uuid.UUID) error
}
