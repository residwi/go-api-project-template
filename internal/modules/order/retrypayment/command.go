// Package retrypayment lets a customer re-request payment on an order still
// awaiting one, e.g. after a card decline.
package retrypayment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

type Params struct {
	PaymentMethodID string
}

// Result is payment's own charge result: retrying payment is a passthrough to
// InitiatePayment, not a new outcome shape.
type Result = paymentcontract.ChargeResult

type Command struct {
	repo    Repository
	payment PaymentInitiator
}

func New(repo Repository, payment PaymentInitiator) *Command {
	return &Command{repo: repo, payment: payment}
}

func (c *Command) Execute(ctx context.Context, userID, orderID uuid.UUID, p Params) (*Result, error) {
	order, err := c.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}
	if order.Status != domain.StatusAwaitingPayment {
		return nil, apperror.ErrOrderNotPayable
	}

	result, err := c.payment.InitiatePayment(ctx, paymentcontract.ChargeRequest{
		OrderID:         order.ID,
		Amount:          order.Total,
		PaymentMethodID: p.PaymentMethodID,
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}
