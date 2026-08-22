package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product"
)

type ProductLookup interface {
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]product.Info, error)
}
