// Package contract is order's published surface. It imports no module and no
// platform package, so payment's config validation can read the threshold below
// without pulling order's implementation into config loading.
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

// Order is order's published read shape. GetSnapshot fills every field, for
// payment to decide a charge or refund outcome. GetInfo -- shipping's
// ownership check -- fills only ID, UserID and Status, since that is all an
// ownership check needs.
type Order struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Total  money.Money
	Status string
	// CouponCode flattens order's internal *string: payment has no use for the
	// pointer, only for whether a coupon is present.
	CouponCode string
	// Persisted by order, not re-derived from Status. Payment reads them to choose
	// restock vs release, and to skip a reversal that already happened rather than
	// double-releasing another order's stock.
	StockDeducted bool
	StockReversed bool
	// Order owns the mapping from its status enum; payment reads the flag rather
	// than re-deriving order semantics.
	Dispatched bool
}
