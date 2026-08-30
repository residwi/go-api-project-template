package payment

import (
	"context"

	"github.com/google/uuid"
)

type Queue interface {
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
	CancelPendingForOrder(ctx context.Context, orderID uuid.UUID) error
}
