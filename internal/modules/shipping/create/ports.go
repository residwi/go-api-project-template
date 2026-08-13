package create

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

type OrderPort interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
}
