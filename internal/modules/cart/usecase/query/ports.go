package query

import (
	"context"

	"github.com/google/uuid"

	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
)

type ProductLookup interface {
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error)
}
