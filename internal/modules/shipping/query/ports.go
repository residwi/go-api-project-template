package query

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
)

// OrderProvider is satisfied directly by order.Service.GetInfo -- no adapter.
// GetInfo, not GetSnapshot: it fills ID, UserID and Status, which is all an
// ownership check needs, and GetSnapshot leaves both ID and UserID zero.
type OrderProvider interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}
