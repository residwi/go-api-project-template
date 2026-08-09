package create

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

// A business default, not a transport one: a seeder or CLI has no request to
// read a currency from.
const defaultCurrency = "USD"

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

// Slug is not a param: it is always derived server-side from Name via
// slug.MakeOrFallback.

// Params may leave Price's currency empty: Execute denominates it.
// CompareAtPrice is denominated from Price, never independently.
type Params struct {
	CategoryID     *uuid.UUID
	Name           string
	Description    *string
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
}

// Command takes no TxRunner: it writes one row through its own repository,
// then registers a zero inventory level, with nothing else to ask.
type Command struct {
	repo Repository
	reg  InventoryRegistrar
}

func New(repo Repository, reg InventoryRegistrar) *Command {
	return &Command{repo: repo, reg: reg}
}

func (c *Command) Execute(ctx context.Context, p Params) (*domain.Product, error) {
	price := p.Price
	if price.Currency == "" {
		price.Currency = defaultCurrency
	}

	prod := &domain.Product{
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Slug:           slug.MakeOrFallback(p.Name, "product-"+uuid.New().String()[:8]),
		Description:    p.Description,
		Price:          price,
		CompareAtPrice: denominateLike(p.CompareAtPrice, price),
		SKU:            p.SKU,
		Status:         domain.StatusDraft,
	}

	if p.Status != "" {
		prod.Status = p.Status
	}

	if err := c.repo.Create(ctx, prod); err != nil {
		return nil, err
	}

	if err := c.reg.EnsureLevel(ctx, prod.ID); err != nil {
		return nil, err
	}

	// (0,0) by construction: EnsureLevel just wrote the row and nothing can hold a
	// reservation yet, so reading inventory back would be a round trip for a
	// known value.
	prod.Availability = inventorycontract.Availability{}
	return prod, nil
}
