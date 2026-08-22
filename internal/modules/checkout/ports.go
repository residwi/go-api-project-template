package checkout

import (
	"context"

	"github.com/google/uuid"

	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

// OrderWriter is satisfied by order's place use case. Everything through the
// order-writing transaction stays there; checkout only adds the payment tail.
type OrderWriter interface {
	Place(
		ctx context.Context,
		userID uuid.UUID,
		in orderdomain.NewOrder,
		idempotencyKey string,
	) (*orderdomain.Order, error)
}

type PaymentCharger interface {
	Charge(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}
