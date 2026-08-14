package transition

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (a *UseCase) Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error {
	return a.repo.Apply(ctx, orderID, t)
}

func (a *UseCase) UpdateStatus(ctx context.Context, orderID uuid.UUID, from, to domain.Status) error {
	return a.repo.UpdateStatus(ctx, orderID, from, to)
}

func (a *UseCase) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaymentProcessingTransition)
}

func (a *UseCase) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.AwaitingPaymentTransition)
}

func (a *UseCase) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaidTransition)
}

func (a *UseCase) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedAfterChargeTransition)
}

func (a *UseCase) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedCompensatingTransition)
}

func (a *UseCase) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.RefundTransition)
}

func (a *UseCase) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.ShippedTransition)
}

func (a *UseCase) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.DeliveredTransition)
}
