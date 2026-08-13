package remove

import (
	"context"

	"github.com/google/uuid"
)

type StatusInvalidator interface {
	Invalidate(ctx context.Context, userID uuid.UUID) error
}
