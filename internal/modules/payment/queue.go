package payment

import (
	"context"

	"github.com/google/uuid"
)

type RefundQueue interface {
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
	CancelPendingForOrder(ctx context.Context, orderID uuid.UUID) error
}
