// Package domain holds order's aggregate and its state machine. It is
// module-private: what leaves order leaves through a slice's return type or
// contract/.
package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

type Status string

const (
	StatusAwaitingPayment   Status = "awaiting_payment"
	StatusPaymentProcessing Status = "payment_processing"
	StatusPaid              Status = "paid"
	StatusProcessing        Status = "processing"
	StatusShipped           Status = "shipped"
	StatusDelivered         Status = "delivered"
	StatusCancelled         Status = "cancelled"
	StatusExpired           Status = "expired"
	StatusRefunded          Status = "refunded"
	StatusFulfillmentFailed Status = "fulfillment_failed"
)

type Address struct {
	Street  string
	City    string
	State   string
	ZipCode string
	Country string
}

type Order struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	IdempotencyKey  string
	RequestHash     string
	Status          Status
	Subtotal        money.Money
	Discount        money.Money
	Total           money.Money
	CouponCode      *string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
	// Persisted rather than derived from Status, because fulfillment_failed is
	// reachable from both reserved-only and deducted states. A reversal reads them
	// to choose release vs restock vs no-op.
	StockDeducted bool
	StockReversed bool
	Items         []Item
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Dispatched reports whether the goods have left. Payment reads this through
// contract.Order to decide whether a refund restocks.
func (o *Order) Dispatched() bool {
	return o.Status == StatusShipped || o.Status == StatusDelivered
}

type Item struct {
	ID          uuid.UUID
	OrderID     uuid.UUID
	ProductID   uuid.UUID
	ProductName string
	Price       money.Money
	Quantity    int
	Subtotal    money.Money
	CreatedAt   time.Time
}
