package cart

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/product"
)

type ProductLookup interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*product.Info, error)
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]product.Info, error)
}
