package register

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
