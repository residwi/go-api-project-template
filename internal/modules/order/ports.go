package order

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/money"
)

type CartProvider interface {
	// LockCart serializes concurrent checkouts of one cart. Returns
	// apperror.ErrNotFound when the user has no cart.
	LockCart(ctx context.Context, userID uuid.UUID) error
	GetCart(ctx context.Context, userID uuid.UUID) (*CartSnapshot, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

type CartSnapshot struct {
	ID    uuid.UUID
	Items []CartSnapshotItem
}

type CartSnapshotItem struct {
	ProductID uuid.UUID
	Quantity  int
	Name      string
	Price     money.Money
	Status    string
}

type InventoryReserver interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, params InitiatePaymentParams) (PaymentResult, error)
}

type InitiatePaymentParams struct {
	OrderID         uuid.UUID
	Amount          money.Money
	PaymentMethodID string
}

type PaymentResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}

type PaymentJobCanceller interface {
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
}

type CouponReserver interface {
	Reserve(
		ctx context.Context,
		code string,
		userID uuid.UUID,
		orderID uuid.UUID,
		orderSubtotal int64,
	) (discountAmount int64, err error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}
