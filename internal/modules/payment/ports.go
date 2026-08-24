package payment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
)

type Gateway interface {
	Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error)
	Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error)
}

type Orders interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type Inventory interface {
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventory.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}
