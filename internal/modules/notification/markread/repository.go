package markread

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
}
