package order

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

const StaleProcessingThreshold = 15 * time.Minute

type Snapshot struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Total  money.Money
	Status string
}

type FulfilmentSnapshot struct {
	Snapshot

	CouponCode    string
	StockDeducted bool
	StockReversed bool
	Dispatched    bool
}

type Address struct {
	Street  string
	City    string
	State   string
	ZipCode string
	Country string
}

type NewOrder struct {
	CouponCode      *string
	ShippingAddress *Address
	BillingAddress  *Address
	Notes           string
}
