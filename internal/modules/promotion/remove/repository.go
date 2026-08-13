package remove

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Delete(ctx context.Context, id uuid.UUID) error
}
