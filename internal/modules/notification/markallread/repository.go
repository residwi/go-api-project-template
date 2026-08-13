package markallread

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}
