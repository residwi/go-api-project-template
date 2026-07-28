package cart

import (
	"time"

	"github.com/google/uuid"
)

type Cart struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Items     []Item
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Item struct {
	ID        uuid.UUID
	CartID    uuid.UUID
	ProductID uuid.UUID
	Quantity  int
	Product   *Product
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Product struct {
	Name     string
	Price    int64
	Currency string
	Stock    int
	Status   string
}

// Sellable reports whether this line can still be purchased. A product that
// was archived, unpublished, or removed after being added to the cart stops
// being sellable, but the line stays visible -- callers decide how to show
// that, not whether to hide it.
func (p *Product) Sellable() bool {
	return p.Status == productStatusPublished
}
