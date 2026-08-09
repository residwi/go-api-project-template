package charge

import (
	"context"

	"github.com/google/uuid"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

// Gateway is narrower than the full gateway.Gateway: InitiatePayment and
// ProcessCharge only ever charge, never refund. Declaring it here rather than
// depending on gateway.Gateway directly is also what gives charge's own
// mockery-generated MockGateway somewhere to live -- a mock cannot be written
// into a package that does not declare the interface it mocks.
type Gateway interface {
	Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error)
}

// OrderUpdater is intent methods, so charge never imports order:
// order.Module's Transition delegators satisfy each by name-match. Five of
// order's seven Mark*/CancelUnpaid methods land here -- every one a charge
// attempt, successful or not, ever drives. MarkRefunded is refund's alone,
// and CancelUnpaid is webhook's.
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
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
}

// JobStore reaches jobs/ through this narrow port instead of importing it:
// jobs owns every operation on payment_jobs, so charge settles its own job
// row and enqueues a follow-up refund only through these three methods.
type JobStore interface {
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error
	UpdateJob(ctx context.Context, job *domain.Job) error
	EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error
}
