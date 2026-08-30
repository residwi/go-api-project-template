package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

type RefundArgs struct {
	PaymentID uuid.UUID
	OrderID   uuid.UUID
}

func (RefundArgs) Kind() string { return "payment.refund" }

func (RefundArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "payment",
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByQueue: true},
	}
}

type Refunder interface {
	SettleRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
}

type RefundWorker struct {
	river.WorkerDefaults[RefundArgs]

	service Refunder
	timeout time.Duration
}

func NewRefundWorker(service Refunder, timeout time.Duration) *RefundWorker {
	return &RefundWorker{service: service, timeout: timeout}
}

func (w *RefundWorker) Timeout(*river.Job[RefundArgs]) time.Duration { return w.timeout }

func (w *RefundWorker) Work(ctx context.Context, job *river.Job[RefundArgs]) error {
	err := w.service.SettleRefund(ctx, job.Args.PaymentID, job.Args.OrderID)
	if errors.Is(err, payment.ErrNotRefundable) {
		return river.JobCancel(err)
	}
	return err
}
