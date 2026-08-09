package webhook

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// OrderUpdater is the one intent method webhook needs from order: on a
// failed/cancelled/expired gateway event, order.Cancel owns the entire
// reversal (stock, coupon, guarded status CAS) behind this single call.
type OrderUpdater interface {
	// Returns a wrapped apperror.ErrBadRequest when the order is no longer
	// cancellable, e.g. already paid by a concurrent charge.
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

// PaymentFinalizer reaches charge/ through this narrow port instead of
// importing it: charge owns the finalize dance because InitiatePayment and
// its own worker-driven ProcessCharge both need it, and webhook's success
// event needs the exact same logic.
type PaymentFinalizer interface {
	FinalizePaymentSuccess(ctx context.Context, job domain.Job) error
	RunCompensatingRefund(ctx context.Context, job domain.Job)
}

// JobStore reaches jobs/ for the two queue operations a webhook triggers: a
// failure event cancels any pending job for the order, and a success event
// completes whatever job the gateway's own async processing may have raced.
type JobStore interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
	MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error
}
