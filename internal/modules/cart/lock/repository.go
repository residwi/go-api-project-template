package lock

import (
	"context"

	"github.com/google/uuid"
)

// Repository is lock's own storage. Its only implementation is
// lock/postgres, constructed in cart/module.go.
type Repository interface {
	GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
