package webhook

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type OrderUpdater interface {
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

type PaymentFinalizer interface {
	FinalizePaymentSuccess(ctx context.Context, job domain.Job) error
	RunCompensatingRefund(ctx context.Context, job domain.Job)
}

type JobStore interface {
	CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error
	MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action domain.JobAction) error
}
