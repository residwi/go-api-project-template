package cart

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
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
	Name   string
	Price  money.Money
	Stock  int
	Status string
}

// Sellable goes false once the product is archived, unpublished or removed, but
// the line stays visible: callers decide how to show it, not whether to.
func (p *Product) Sellable() bool {
	return p.Status == productStatusPublished
}

// Total sums the sellable lines only, so an unsellable line's currency cannot
// make an otherwise single-currency cart unsummable. A nil Product counts the
// same way. An empty cart totals the zero Money, which publishes as the
// `total: 0` clients have always seen. See ARCHITECTURE.md §10.
func (c *Cart) Total() (money.Money, error) {
	var total money.Money
	seeded := false
	for _, it := range c.Items {
		if it.Product == nil || !it.Product.Sellable() {
			continue
		}
		if !seeded {
			// Seeded from the first sellable line so it folds through the same Add.
			total = money.New(0, it.Product.Price.Currency)
			seeded = true
		}
		sum, err := total.Add(it.Product.Price.MulQty(it.Quantity))
		if err != nil {
			return money.Money{}, fmt.Errorf("%w: cart contains mixed currencies: %w", apperror.ErrBadRequest, err)
		}
		total = sum
	}
	return total, nil
}
