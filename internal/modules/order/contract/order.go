package contract

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// StaleProcessingThreshold is how long an order may sit in payment_processing
// before the recovery sweep reverts it. A charge job's lease must expire well
// before this, or the sweep reverts an order whose charge is still running --
// which is why payment validates its own timeouts against this value.
const StaleProcessingThreshold = 15 * time.Minute

type Order struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Total         money.Money
	Status        string
	CouponCode    string
	StockDeducted bool
	StockReversed bool
	Dispatched    bool
}
