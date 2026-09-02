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
	StockDeducted   bool
	StockReversed   bool
	Items           []Item
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

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

type NewOrder struct {
	CouponCode      *string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
}
