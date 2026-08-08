package reserve

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// Command writes a usage row inside the caller's transaction and Release
// deletes it, so -- unlike apply -- it takes a database.TxRunner. Reserve and
// Release are named for what order.CouponReserver and payment.CouponReleaser
// declare, not for this package's own convention, so that both are satisfied
// by name-match with no adapter.
type Command struct {
	repo Repository
	tx   database.TxRunner
}

func New(repo Repository, tx database.TxRunner) *Command {
	return &Command{repo: repo, tx: tx}
}

func (c *Command) Reserve(
	ctx context.Context,
	code string,
	userID, orderID uuid.UUID,
	orderSubtotal int64,
) (int64, error) {
	var discountAmount int64

	err := c.tx.Run(ctx, func(ctx context.Context) error {
		promo, err := c.repo.GetByCode(ctx, code)
		if err != nil {
			return err
		}

		if err := domain.ValidatePromotion(promo, orderSubtotal); err != nil {
			return err
		}

		discountAmount = domain.ComputeDiscount(promo, orderSubtotal)

		if err := c.repo.ApplyPromotion(ctx, promo.ID); err != nil {
			return err
		}

		usage := &domain.CouponUsage{
			CouponID: promo.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: discountAmount,
		}
		return c.repo.CreateUsage(ctx, usage)
	})

	return discountAmount, err
}

func (c *Command) Release(ctx context.Context, orderID uuid.UUID) error {
	return c.tx.Run(ctx, func(ctx context.Context) error {
		usage, err := c.repo.DeleteUsageByOrderID(ctx, orderID)
		if err != nil {
			if errors.Is(err, apperror.ErrNotFound) {
				return nil
			}
			return err
		}

		return c.repo.ReleasePromotion(ctx, usage.CouponID)
	})
}
