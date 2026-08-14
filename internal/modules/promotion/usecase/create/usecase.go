package create

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Params struct {
	Code           string
	Type           domain.Type
	Value          int64
	MinOrderAmount int64
	MaxDiscount    *int64
	MaxUses        *int
	StartsAt       time.Time
	ExpiresAt      time.Time
	Active         bool
}

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) Execute(ctx context.Context, p Params) (*domain.Promotion, error) {
	promo := &domain.Promotion{
		Code:           p.Code,
		Type:           p.Type,
		Value:          p.Value,
		MinOrderAmount: p.MinOrderAmount,
		MaxDiscount:    p.MaxDiscount,
		MaxUses:        p.MaxUses,
		StartsAt:       p.StartsAt,
		ExpiresAt:      p.ExpiresAt,
		Active:         p.Active,
	}

	if err := domain.ValidatePercentageValue(promo.Type, promo.Value); err != nil {
		return nil, err
	}

	if err := c.repo.Create(ctx, promo); err != nil {
		return nil, err
	}

	return promo, nil
}
