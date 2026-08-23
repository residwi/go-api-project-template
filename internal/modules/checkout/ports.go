package checkout

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

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

type OrderSnapshotReader interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
}

type PaymentAttemptClaimer interface {
	BeginPaymentAttempt(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
}

type OrderCanceller interface {
	CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error
}

type PaymentJobCanceller interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}
