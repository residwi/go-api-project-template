package cancel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type UseCase struct {
	repo       Repository
	tx         database.TxRunner
	transition TransitionApplier
	inventory  InventoryRestorer
	coupons    CouponReleaser
	logger     *slog.Logger
}

func New(
	repo Repository,
	tx database.TxRunner,
	transition TransitionApplier,
	inventory InventoryRestorer,
	coupons CouponReleaser,
	log *slog.Logger,
) *UseCase {
	return &UseCase{
		repo:       repo,
		tx:         tx,
		transition: transition,
		inventory:  inventory,
		coupons:    coupons,
		logger:     log,
	}
}

func (c *UseCase) CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error {
	order, err := c.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if order.UserID != userID {
		return apperror.ErrNotFound
	}

	if order.Status == domain.StatusPaymentProcessing {
		return apperror.ErrOrderCharging
	}

	return c.cancelWithReversal(ctx, order)
}

func (c *UseCase) CancelUnpaid(ctx context.Context, orderID uuid.UUID) error {
	order, err := c.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	return c.cancelWithReversal(ctx, order)
}

//nolint:gocognit // the single cancel path: guarded status CAS, conditional stock reversal (release vs restock vs skip), and best-effort coupon release
func (c *UseCase) cancelWithReversal(ctx context.Context, order *domain.Order) error {
	return c.tx.Run(ctx, func(txCtx context.Context) error {
		if txErr := c.transition.Apply(txCtx, order.ID, domain.CancelledTransition); txErr != nil {
			if errors.Is(txErr, apperror.ErrConflict) {
				return fmt.Errorf("%w: cannot cancel order in status %s", apperror.ErrBadRequest, order.Status)
			}
			return txErr
		}

		items, txErr := c.repo.ListItemsByOrderID(txCtx, order.ID)
		if txErr != nil {
			return txErr
		}
		if len(items) > 0 && !order.StockReversed {
			releases := make(map[uuid.UUID]int, len(items))
			for _, item := range items {
				releases[item.ProductID] = item.Quantity
			}
			releaseErr := c.inventory.Restore(txCtx, releases, stockStateFor(order.StockDeducted))
			if releaseErr != nil {
				return fmt.Errorf("restoring inventory on cancel: %w", releaseErr)
			}
		}

		if c.coupons != nil && order.CouponCode != nil && *order.CouponCode != "" {
			if releaseErr := c.coupons.Release(txCtx, order.ID); releaseErr != nil {
				c.logger.WarnContext(
					txCtx,
					"failed to release coupon on cancel",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		return nil
	})
}

func stockStateFor(deducted bool) inventory.StockState {
	if deducted {
		return inventory.Deducted
	}
	return inventory.Reserved
}
