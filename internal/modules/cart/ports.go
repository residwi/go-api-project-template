package cart

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product"
)

// ProductLookup is satisfied by product's Service. Add and UpdateQuantity
// need only GetInfo, to validate a single line; Get and Snapshot need
// GetInfoByIDs, to enrich a whole cart in one round trip.
type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*product.Info, error)
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]product.Info, error)
}
