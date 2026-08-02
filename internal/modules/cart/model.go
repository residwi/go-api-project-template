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
	Name string
	// Price carries its own currency, because a cart's lines are not guaranteed to
	// share one: prices are per-product, and nothing prevents adding two products
	// priced in different currencies to the same cart. Cart.Total is where that
	// collision is detected rather than summed over.
	Price  money.Money
	Stock  int
	Status string
}

// Sellable reports whether this line can still be purchased. A product that
// was archived, unpublished, or removed after being added to the cart stops
// being sellable, but the line stays visible -- callers decide how to show
// that, not whether to hide it.
func (p *Product) Sellable() bool {
	return p.Status == productStatusPublished
}

// Total is what the cart's sellable lines come to.
//
// This is the cart's rule, not its transport's; see ARCHITECTURE.md §10 for why
// it moved out of the http adapter. Product.Sellable() moved back here for the
// same reason.
//
// Only sellable lines count, exactly as before. A product archived, unpublished
// or withdrawn after being added stays visible so the customer can see why their
// total changed, but it is not something they can be charged for -- so its price
// is excluded, and its currency cannot make an otherwise single-currency cart
// unsummable. A nil Product is treated the same way: Service.GetCart never
// leaves one, but a Cart read straight from the repository has none yet, and an
// un-priced line is not chargeable either.
//
// A cart with no sellable lines totals the zero Money -- zero, in no currency.
// There is nothing to charge and no denomination to charge it in, and a caller
// publishing the bare amount gets the `total: 0` an empty cart has always
// returned.
//
// Mixed currencies are refused rather than summed into a plausible-looking wrong
// number; see ARCHITECTURE.md §10 for why the error wraps two sentinels and how
// this keeps GET /cart and PlaceOrder in agreement.
func (c *Cart) Total() (money.Money, error) {
	var total money.Money
	seeded := false
	for _, it := range c.Items {
		if it.Product == nil || !it.Product.Sellable() {
			continue
		}
		if !seeded {
			// Seed with a zero denominated in the first sellable line's currency, so
			// that line folds through the same Add as every other one.
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
