package order

import (
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
	// StockDeducted reports whether the order's reserved stock has been deducted
	// (sold); StockReversed reports whether its inventory hold has already been
	// released or restocked. Both are persisted and set atomically with the
	// transition that changes them, because fulfillment_failed is reachable from
	// both reserved-only and deducted states and so cannot be classified from
	// Status alone. A reversal reads these to choose release vs restock vs no-op.
	StockDeducted bool      `json:"-"`
	StockReversed bool      `json:"-"`
	Items         []Item    `json:"items,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
