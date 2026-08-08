package apply

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

// Command holds no transaction runner: it reads a promotion and computes a
// discount, writing nothing. That absence is the structural proof that apply
// is a preview, not a claim -- reserve is the slice that writes a usage row.
type Command struct {
	repo Repository
}

func New(repo Repository) *Command {
	return &Command{repo: repo}
}

func (c *Command) Execute(ctx context.Context, code string, orderAmount int64) (int64, error) {
	promo, err := c.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, err
	}

	if err := domain.ValidatePromotion(promo, orderAmount); err != nil {
		return 0, err
	}

	return domain.ComputeDiscount(promo, orderAmount), nil
}
