package update

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Params struct {
	Code           string
	Type           domain.Type
	Value          *int64
	MinOrderAmount *int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       *time.Time
	ExpiresAt      *time.Time
	Active         *bool
}

// Command takes no TxRunner: it loads one row through its own repository,
// patches it and writes it back, with nothing else to ask.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID, p Params) (*domain.Promotion, error) {
	promo, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.Code != "" {
		promo.Code = p.Code
	}
	if p.Type != "" {
		promo.Type = p.Type
	}
	if p.Value != nil {
		promo.Value = *p.Value
	}
	if p.MinOrderAmount != nil {
		promo.MinOrderAmount = *p.MinOrderAmount
	}
	if p.MaxDiscount != nil {
		promo.MaxDiscount = p.MaxDiscount
	}
	if p.MaxUses != nil {
		promo.MaxUses = p.MaxUses
	}
	if p.StartsAt != nil {
		promo.StartsAt = *p.StartsAt
	}
	if p.ExpiresAt != nil {
		promo.ExpiresAt = *p.ExpiresAt
	}
	if p.Active != nil {
		promo.Active = *p.Active
	}

	// Type and/or Value may be partially supplied; validate the final
	// persisted combination, not just the incoming fields.
	if err := domain.ValidatePercentageValue(promo.Type, promo.Value); err != nil {
		return nil, err
	}

	if err := c.repo.Update(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}
