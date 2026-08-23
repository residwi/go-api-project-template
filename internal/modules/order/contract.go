package order

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
)

// StaleProcessingThreshold is how long an order may sit in payment_processing
// before the recovery sweep reverts it. A charge job's lease must expire well
// before this, or the sweep reverts an order whose charge is still running --
// which is why payment validates its own timeouts against this value.
const StaleProcessingThreshold = 15 * time.Minute

// Snapshot is the flat order projection other modules read across a port:
// payment for its charge and refund decisions, checkout for retry ownership,
// shipping for the order's owner and status. Status is a string rather than
// domain.Status so a consumer needs nothing from order's domain package to
// compare it.
type Snapshot struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Total         money.Money
	Status        string
	CouponCode    string
	StockDeducted bool
	StockReversed bool
	Dispatched    bool
}
