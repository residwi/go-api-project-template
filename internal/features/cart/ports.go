package cart

import (
	"context"

	"github.com/google/uuid"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

type ProductLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ProductInfo, error)
}

type ProductInfo struct {
	ID        uuid.UUID
	Name      string
	Price     int64
	Currency  string
	Status    string
	Available int
}
