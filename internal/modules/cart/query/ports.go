package query

import (
	"context"

	"github.com/google/uuid"

	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
)

// ProductLookup answers for a whole cart at once, and reports a withdrawn
// product as unavailable rather than as its stale published status. Kept as
// its own copy rather than sharing one with add/ or updatequantity/, because
// a slice may not import a sibling's port.
type ProductLookup interface {
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error)
}
