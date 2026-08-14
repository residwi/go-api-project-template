package create

import (
	"context"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

const defaultCurrency = "USD"

func denominateLike(amount *money.Money, price money.Money) *money.Money {
	if amount == nil {
		return nil
	}
	restated := money.New(amount.Amount, price.Currency)
	return &restated
}

type Params struct {
	CategoryID     *uuid.UUID
	Name           string
	Description    *string
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
}

type UseCase struct {
	repo Repository
	reg  InventoryRegistrar
}

func New(repo Repository, reg InventoryRegistrar) *UseCase {
	return &UseCase{repo: repo, reg: reg}
}

func (c *UseCase) Execute(ctx context.Context, p Params) (*domain.Product, error) {
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

	prod.Availability = inventorycontract.Availability{}
	return prod, nil
}
