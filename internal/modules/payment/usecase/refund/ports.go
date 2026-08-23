package refund

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

type Gateway interface {
	Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error)
}

type OrderUpdater interface {
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
}

type OrderGetter interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
}

type OrderItemsGetter interface {
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventory.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

type JobStore interface {
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
}
