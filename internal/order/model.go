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
	ID             uuid.UUID
	UserID         uuid.UUID
	IdempotencyKey string
	RequestHash    string
	Status         Status
	// Subtotal, Discount and Total each carry their own currency, but all three
	// are always denominated the same: an order is placed from a single-currency
	// cart, and the orders table stores one currency column for all three amounts.
	// Pairing each amount with it means arithmetic across them cannot silently mix
	// currencies, and the http adapter reads the order's currency off Total.
	Subtotal        money.Money
	Discount        money.Money
	Total           money.Money
	CouponCode      *string
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
	// Price and Subtotal are denominated in the parent order's currency. The
	// order_items table has no currency column of its own -- the currency is
	// stored once, on the order -- so every construction site has to supply it
	// explicitly: at checkout from the cart item's price, and on read from the
	// joined orders row. An Item left with an empty currency would fail to Add
	// into the order total rather than quietly contributing a wrong number.
	Price     money.Money
	Quantity  int
	Subtotal  money.Money
	CreatedAt time.Time
}
