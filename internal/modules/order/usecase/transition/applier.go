package transition

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type Applier struct {
	repo Repository
}

func New(repo Repository) *Applier {
	return &Applier{repo: repo}
}

func (a *Applier) Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error {
	return a.repo.Apply(ctx, orderID, t)
}

func (a *Applier) UpdateStatus(ctx context.Context, orderID uuid.UUID, from, to domain.Status) error {
	return a.repo.UpdateStatus(ctx, orderID, from, to)
}

func (a *Applier) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaymentProcessingTransition)
}

func (a *Applier) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.AwaitingPaymentTransition)
}

func (a *Applier) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaidTransition)
}

func (a *Applier) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedAfterChargeTransition)
}

func (a *Applier) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedCompensatingTransition)
}

func (a *Applier) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.RefundTransition)
}

func (a *Applier) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.ShippedTransition)
}

func (a *Applier) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.DeliveredTransition)
}
