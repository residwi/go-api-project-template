// Package domain holds cart's aggregate and its rules. It is module-private:
// what leaves cart leaves through a slice's return type or contract/.
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
)

// StatusPublished is the only product status a cart line may be added or
// bumped against. add/ and updatequantity/ both guard on it before touching
// storage; Sellable below reuses it so a line's display state and its
// admission rule can never drift apart.
const StatusPublished = "published"

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
	return p.Status == StatusPublished
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
