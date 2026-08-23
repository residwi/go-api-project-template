package payment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
)

// Gateway is charge's and refund's Gateway ports merged: Charge and Refund
// are different methods, so there was nothing to deduplicate, only to
// declare once instead of twice.
type Gateway interface {
	Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error)
	Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error)
}

// OrderTransition is charge's OrderUpdater (five methods) plus refund's
// OrderUpdater (MarkRefunded). Every method is an intent, and order.Service
// turns each into a named domain.Transition, per AGENTS.md rule 14.
type OrderTransition interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
}

// OrderCanceller stays a separate port from OrderTransition even though one
// order.Service now satisfies both: cancelling and transitioning are different
// asks, and a port names what its caller needs, not what the producer happens
// to offer.
type OrderCanceller interface {
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

// OrderReader is charge's OrderGetter+OrderItemsGetter and refund's
// OrderGetter+OrderItemsGetter, all four already the same two methods.
type OrderReader interface {
	Snapshot(ctx context.Context, orderID uuid.UUID) (order.Snapshot, error)
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type InventoryDeductor interface {
	Deduct(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventory.StockState) error
}

// CouponPort was refund's CouponReleaser.
type CouponPort interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}
