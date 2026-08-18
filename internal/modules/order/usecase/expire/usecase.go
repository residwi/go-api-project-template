package expire

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const housekeepingBatchLimit = 20

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
	return &UseCase{repo: repo, tx: tx, transition: transition, inventory: inventory, coupons: coupons, logger: log}
}

func (c *UseCase) ExpireStale(ctx context.Context) error {
	orders, err := c.repo.GetExpiredOrders(ctx, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting expired orders: %w", err)
	}
	for _, o := range orders {
		if err := c.expireOne(ctx, o); err != nil {
			c.logger.ErrorContext(
				ctx,
				"failed to expire order",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func (c *UseCase) expireOne(ctx context.Context, o domain.Order) error {
	return c.tx.Run(ctx, func(txCtx context.Context) error {
		if err := c.transition.Apply(txCtx, o.ID, domain.ExpiredTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				return nil
			}
			return err
		}
		return c.releaseOrderHolds(txCtx, o)
	})
}

func (c *UseCase) releaseOrderHolds(ctx context.Context, o domain.Order) error {
	items, err := c.repo.ListItemsByOrderID(ctx, o.ID)
	if err != nil {
		return err
	}
	if len(items) > 0 && !o.StockReversed {
		releases := make(map[uuid.UUID]int, len(items))
		for _, item := range items {
			releases[item.ProductID] = item.Quantity
		}
		if err := c.inventory.Restore(ctx, releases, stockStateFor(o.StockDeducted)); err != nil {
			return fmt.Errorf("restoring inventory on expire: %w", err)
		}
	}

	if c.coupons != nil && o.CouponCode != nil && *o.CouponCode != "" {
		if err := c.coupons.Release(ctx, o.ID); err != nil {
			c.logger.WarnContext(
				ctx,
				"failed to release coupon on expire",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func stockStateFor(deducted bool) inventorycontract.StockState {
	if deducted {
		return inventorycontract.Deducted
	}
	return inventorycontract.Reserved
}
