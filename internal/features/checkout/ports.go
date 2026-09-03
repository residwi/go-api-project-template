package checkout

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
)

type Orders interface {
	Place(
		ctx context.Context,
		userID uuid.UUID,
		in order.NewOrder,
		idempotencyKey string,
	) (placed *order.Snapshot, created bool, err error)
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
	BeginPaymentAttempt(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error
}

type Payments interface {
	Charge(ctx context.Context, p payment.ChargeRequest) (payment.ChargeResult, error)
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
}
