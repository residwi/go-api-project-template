package query

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

type OrderGetter interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}
