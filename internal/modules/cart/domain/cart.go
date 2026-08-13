package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
)

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

func (p *Product) Sellable() bool {
	return p.Status == StatusPublished
}

func (c *Cart) Total() (money.Money, error) {
	var total money.Money
	seeded := false
	for _, it := range c.Items {
		if it.Product == nil || !it.Product.Sellable() {
			continue
		}
		if !seeded {
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
