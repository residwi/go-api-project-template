package cart

import (
	"context"

	"github.com/google/uuid"

	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
)

type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*productcontract.Product, error)
	// GetInfoByIDs answers for a whole cart at once, and reports a withdrawn
	// product as unavailable rather than as its stale published status.
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error)
}
