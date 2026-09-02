package order

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/money"
)

const StaleProcessingThreshold = 15 * time.Minute

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
