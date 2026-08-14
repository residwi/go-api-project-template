package add

import (
	"context"

	"github.com/google/uuid"

	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
)

type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*productcontract.Product, error)
}
