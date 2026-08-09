package refund

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

// Gateway is narrower than the full gateway.Gateway: Execute and ProcessRefund
// only ever refund, never charge. Declaring it here rather than depending on
// gateway.Gateway directly is also what gives refund's own
// mockery-generated MockGateway somewhere to live -- a mock cannot be written
// into a package that does not declare the interface it mocks.
type Gateway interface {
	Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error)
}

// OrderUpdater is intent methods, so refund never imports order:
// order.Module's Transition delegator satisfies it by name-match. MarkRefunded
// is refund's alone -- none of payment's other slices ever mark a refund.
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
	// Inventory owns the release-vs-restock choice; refund supplies only the
	// order's persisted fact, never the mechanics.
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

// JobStore reaches jobs/ through this narrow port instead of importing it:
// jobs owns every operation on payment_jobs, so refund enqueues its own
// initial job and settles its own retry bookkeeping only through these three
// methods.
type JobStore interface {
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
}
