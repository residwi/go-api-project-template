package retrypayment

import (
	"context"

	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

// PaymentInitiator is set after construction by SetPaymentDeps: payment is not
// sliced yet, and order/payment need each other at construction time, so
// bootstrap wires this one after both exist.
type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}
