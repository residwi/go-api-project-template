package add

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product"
)

type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*product.Info, error)
}
