package register

import (
	"context"

	"github.com/google/uuid"
)

// Repository is register's own storage. Its only implementation is
// register/postgres, constructed in inventory/module.go.
type Repository interface {
	EnsureLevel(ctx context.Context, productID uuid.UUID) error
}
