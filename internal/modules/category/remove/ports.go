package remove

import (
	"context"

	"github.com/google/uuid"
)

type ProductCounter interface {
	CountPublished(ctx context.Context, categoryID uuid.UUID) (int, error)
}
