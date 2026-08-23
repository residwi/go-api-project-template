package checkout

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

// OrderWriter is satisfied by order's place use case. Everything through the
// order-writing transaction stays there; checkout only adds the payment tail.
type OrderWriter interface {
	Place(
		ctx context.Context,
		userID uuid.UUID,
		in orderdomain.NewOrder,
		idempotencyKey string,
	) (*orderdomain.Order, error)
}

type PaymentCharger interface {
	Charge(ctx context.Context, p payment.ChargeRequest) (payment.ChargeResult, error)
}

// OrderSnapshotReader is satisfied by order's query use case. contract.Order
// carries every field retry needs, so no order write port is required.
type OrderSnapshotReader interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

type OrderCanceller interface {
	CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}
