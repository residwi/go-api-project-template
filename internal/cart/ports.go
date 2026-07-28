package cart

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

type ProductLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ProductInfo, error)
	// GetByIDs answers for a whole cart in one call. It returns soft-deleted and
	// unpublished products too, carrying Status -- cart decides whether to show a
	// line as unavailable, because the display rule is cart's, not product's.
	GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductInfo, error)
}

type ProductInfo struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
