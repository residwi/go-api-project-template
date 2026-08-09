package remove

import (
	"context"

	"github.com/google/uuid"
)

// Repository is remove's own storage. Its only implementation is
// remove/postgres, constructed in product/module.go.
type Repository interface {
	Delete(ctx context.Context, id uuid.UUID) error
}
