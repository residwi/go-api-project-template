package payment

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/jobs"
)

type RefundJob struct {
	PaymentID uuid.UUID
	OrderID   uuid.UUID

	svc *Service
}

func NewRefundJob(s *Service) RefundJob { return RefundJob{svc: s} }

func (RefundJob) Kind() string { return "payment.refund" }

func (j RefundJob) Run(ctx context.Context) error {
	return j.svc.runRefund(ctx, j.PaymentID, j.OrderID)
}

func (s *Service) enqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	return jobs.Enqueue(ctx, s.queue, RefundJob{PaymentID: paymentID, OrderID: orderID}, jobs.Keys{
		Dedup: "payment.refund:" + paymentID.String(),
		Group: "order:" + orderID.String(),
	})
}

func (s *Service) CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error {
	if _, err := s.queue.CancelByGroupKey(ctx, "order:"+orderID.String()); err != nil {
		return fmt.Errorf("cancelling payment jobs for order %s: %w", orderID, err)
	}
	return nil
}
