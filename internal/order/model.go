package order

import (
	"time"

	"github.com/google/uuid"
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
	SubtotalAmount  int64
	DiscountAmount  int64
	TotalAmount     int64
	CouponCode      *string
	Currency        string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
	// StockDeducted reports whether the order's reserved stock has been deducted
	// (sold); StockReversed reports whether its inventory hold has already been
	// released or restocked. Both are persisted and set atomically with the
	// transition that changes them, because fulfillment_failed is reachable from
	// both reserved-only and deducted states and so cannot be classified from
	// Status alone. A reversal reads these to choose release vs restock vs no-op.
	StockDeducted bool
	StockReversed bool
	Items         []Item
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PlaceResult is PlaceOrder's return value. It is a core type, not a wire
// type: http's mapper decides how (and whether, under an "order" key) it
// reaches the response.
type PlaceResult struct {
	Order *Order
}

type Item struct {
	ID          uuid.UUID
	OrderID     uuid.UUID
	ProductID   uuid.UUID
	ProductName string
	Price       int64
	Quantity    int
	Subtotal    int64
	CreatedAt   time.Time
}
