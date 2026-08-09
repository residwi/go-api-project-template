package updatequantity

import (
	"context"

	"github.com/google/uuid"

	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
)

// ProductLookup is satisfied by product's query slice by name-match. Kept as
// its own copy rather than sharing one with add/ or query/, because a slice
// may not import a sibling's port.
type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*productcontract.Product, error)
}
