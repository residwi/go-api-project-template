package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

// denominateLike restates an optional amount in price's currency, nil passing
// through. products stores one currency for both, so any other would be lost on
// the way to the database and re-read as the price's.
func denominateLike(amount *money.Money, price money.Money) *money.Money {
	if amount == nil {
		return nil
	}
	restated := money.New(amount.Amount, price.Currency)
	return &restated
}

// Params reads nil as "leave it alone". Re-pricing requires a currency too;
// http/handler.go rejects one without the other.
type Params struct {
	CategoryID     *uuid.UUID
	Name           *string
	Description    *string
	Price          *money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         *string
}

// Command takes no TxRunner: it loads one row through its own repository,
// patches it and writes it back, with nothing else to ask.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID, p Params) (*domain.Product, error) {
	prod, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.CategoryID != nil {
		prod.CategoryID = p.CategoryID
	}
	if p.Name != nil {
		prod.Name = *p.Name
		prod.Slug = slug.MakeOrFallback(prod.Name, "product-"+prod.ID.String()[:8])
	}
	if p.Description != nil {
		prod.Description = p.Description
	}
	if p.Price != nil {
		prod.Price = *p.Price
		// The *stored* compare-at price too, or it keeps the old currency. The branch
		// below overwrites this if the caller supplied one.
		prod.CompareAtPrice = denominateLike(prod.CompareAtPrice, prod.Price)
	}
	if p.CompareAtPrice != nil {
		prod.CompareAtPrice = denominateLike(p.CompareAtPrice, prod.Price)
	}
	if p.SKU != nil {
		prod.SKU = p.SKU
	}
	if p.Status != nil {
		prod.Status = *p.Status
	}

	if err := c.repo.Update(ctx, prod); err != nil {
		return nil, err
	}

	return prod, nil
}
