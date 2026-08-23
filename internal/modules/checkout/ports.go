package checkout

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

// OrderWriter is satisfied by order.Service. Everything through the
// order-writing transaction stays there; checkout only adds the payment tail.
// The created flag is what keeps that tail idempotent: false means the
// idempotency key was a replay of an order already placed -- and already
// charged -- so the tail must not run again.
type OrderWriter interface {
	Place(
		ctx context.Context,
		userID uuid.UUID,
		in orderdomain.NewOrder,
		idempotencyKey string,
	) (order *orderdomain.Order, created bool, err error)
}

type PaymentCharger interface {
	Charge(ctx context.Context, p payment.ChargeRequest) (payment.ChargeResult, error)
}

// OrderSnapshotReader is satisfied by order.Service. order.Snapshot carries
// every field retry needs, so no order write port is required.
type OrderSnapshotReader interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
}

type OrderCanceller interface {
	CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}
