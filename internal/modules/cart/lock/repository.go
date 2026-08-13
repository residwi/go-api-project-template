package lock

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetCartForLock(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
