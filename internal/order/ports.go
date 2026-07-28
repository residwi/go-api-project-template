package order

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Ports this feature needs from other features. Each is declared here rather
// than imported, so no feature depends on another's package.

type CartProvider interface {
	// LockCart takes a row lock on the user's cart for the current transaction so
	// concurrent checkouts of the same cart serialize. Returns apperror.ErrNotFound
	// when the user has no cart.
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

// InventoryItem is one product/quantity pair for a batched reserve/restore.
type InventoryItem struct {
	ProductID uuid.UUID
	Quantity  int
}

type InventoryReserver interface {
	ReserveBatch(ctx context.Context, items []InventoryItem) error
	DeductBatch(ctx context.Context, items []InventoryItem) error
	Restore(ctx context.Context, items []InventoryItem, wasDeducted bool) error
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
	Reserve(ctx context.Context, code string, userID uuid.UUID, orderID uuid.UUID, orderSubtotal int64) (discountAmount int64, err error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}
