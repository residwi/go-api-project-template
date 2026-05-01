package order

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/core"
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

var validTransitions = map[Status][]Status{ //nolint:gochecknoglobals // package-level lookup table for state machine
	StatusAwaitingPayment:   {StatusPaymentProcessing, StatusCancelled, StatusExpired},
	StatusPaymentProcessing: {StatusPaid, StatusAwaitingPayment, StatusCancelled, StatusFulfillmentFailed},
	StatusPaid:              {StatusProcessing, StatusRefunded, StatusFulfillmentFailed},
	StatusProcessing:        {StatusShipped, StatusRefunded},
	StatusShipped:           {StatusDelivered, StatusRefunded},
	StatusDelivered:         {StatusRefunded},
	StatusFulfillmentFailed: {StatusRefunded, StatusCancelled},
	StatusCancelled:         {},
	StatusExpired:           {},
	StatusRefunded:          {},
}

func CanTransition(from, to Status) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(targets, to)
}

type Order struct {
	ID              uuid.UUID     `json:"id"`
	UserID          uuid.UUID     `json:"user_id"`
	IdempotencyKey  string        `json:"idempotency_key"`
	RequestHash     string        `json:"-"`
	Status          Status        `json:"status"`
	SubtotalAmount  int64         `json:"subtotal_amount"`
	DiscountAmount  int64         `json:"discount_amount"`
	TotalAmount     int64         `json:"total_amount"`
	CouponCode      *string       `json:"coupon_code,omitempty"`
	Currency        string        `json:"currency"`
	ShippingAddress *core.Address `json:"shipping_address,omitempty"`
	BillingAddress  *core.Address `json:"billing_address,omitempty"`
	Notes           string        `json:"notes,omitempty"`
	Items           []Item        `json:"items,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type Item struct {
	ID          uuid.UUID `json:"id"`
	OrderID     uuid.UUID `json:"-"`
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Quantity    int       `json:"quantity"`
	Subtotal    int64     `json:"subtotal"`
	CreatedAt   time.Time `json:"created_at"`
}
