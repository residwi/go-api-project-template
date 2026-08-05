package order

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

type PlaceResult struct {
	Order *Order
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
