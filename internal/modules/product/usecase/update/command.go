package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

func denominateLike(amount *money.Money, price money.Money) *money.Money {
	if amount == nil {
		return nil
	}
	restated := money.New(amount.Amount, price.Currency)
	return &restated
}

type Params struct {
	CategoryID     *uuid.UUID
	Name           *string
	Description    *string
	Price          *money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         *string
}

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
