package charge

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

type Gateway interface {
	Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error)
}

type OrderUpdater interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
}

type OrderGetter interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

type OrderItemsGetter interface {
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type InventoryDeductor interface {
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
}

type JobStore interface {
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
}
