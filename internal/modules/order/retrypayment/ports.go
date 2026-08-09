package retrypayment

import (
	"context"

	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

// PaymentInitiator is a constructor argument, not a setter: at slice
// granularity the order/payment cycle runs through four packages
// (order/transition, order/query, payment/charge, payment/jobs), not two, so
// bootstrap builds order's and payment's shared reads first, then payment,
// then hands payment.Charge in here at construction time.
type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}
