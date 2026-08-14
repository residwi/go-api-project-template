package apply

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) Execute(ctx context.Context, code string, orderAmount int64) (int64, error) {
	promo, err := c.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, err
	}

	if err := domain.ValidatePromotion(promo, orderAmount); err != nil {
		return 0, err
	}

	return domain.ComputeDiscount(promo, orderAmount), nil
}
