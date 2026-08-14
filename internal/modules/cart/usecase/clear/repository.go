package clear

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Clear(ctx context.Context, userID uuid.UUID) error
}
