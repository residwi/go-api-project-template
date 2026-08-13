package retrypayment

import (
	"context"

	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}
