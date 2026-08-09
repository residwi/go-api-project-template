package place

import (
	"context"

	"github.com/google/uuid"

	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

type CartProvider interface {
	// LockCart serializes concurrent checkouts of one cart. Returns
	// apperror.ErrNotFound when the user has no cart.
	LockCart(ctx context.Context, userID uuid.UUID) error
	GetSnapshot(ctx context.Context, userID uuid.UUID) (*cartcontract.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// InventoryReserver is narrower than order's old whole-service port: place
// never restores stock, only reserves and (for a free order) deducts it.
type InventoryReserver interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
}

// PaymentInitiator is a constructor argument, not a setter: at slice
// granularity the order/payment cycle runs through four packages
// (order/transition, order/query, payment/charge, payment/jobs), not two, so
// bootstrap builds order's and payment's shared reads first, then payment,
// then hands payment.Charge in here at construction time.
type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, p paymentcontract.ChargeRequest) (paymentcontract.ChargeResult, error)
}

// CouponReserver is narrower than order's old whole-service port: place only
// reserves a discount, it never releases one.
type CouponReserver interface {
	Reserve(
		ctx context.Context,
		code string,
		userID uuid.UUID,
		orderID uuid.UUID,
		orderSubtotal int64,
	) (discountAmount int64, err error)
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

// TransitionApplier reaches transition/ through this narrow port instead of
// importing it: finalizeFreeOrder needs only Apply, for the PaidTransition
// case where a coupon covers the whole order and there is nothing to charge.
type TransitionApplier interface {
	Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error
}
