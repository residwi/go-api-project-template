package place

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const productStatusPublished = "published"

type UseCase struct {
	repo          Repository
	tx            database.TxRunner
	locker        CartLocker
	carts         CartReader
	clearer       CartClearer
	reserver      InventoryReserver
	deductor      InventoryDeductor
	coupons       CouponReserver
	notifications NotificationEnqueuer
	transition    TransitionApplier
	logger        *slog.Logger
}

type Deps struct {
	Repo          Repository
	Tx            database.TxRunner
	Locker        CartLocker
	Carts         CartReader
	Clearer       CartClearer
	Reserver      InventoryReserver
	Deductor      InventoryDeductor
	Coupons       CouponReserver
	Notifications NotificationEnqueuer
	Transition    TransitionApplier
	Logger        *slog.Logger
}

func New(d Deps) *UseCase {
	return &UseCase{
		repo:          d.Repo,
		tx:            d.Tx,
		locker:        d.Locker,
		carts:         d.Carts,
		clearer:       d.Clearer,
		reserver:      d.Reserver,
		deductor:      d.Deductor,
		coupons:       d.Coupons,
		notifications: d.Notifications,
		transition:    d.Transition,
		logger:        d.Logger,
	}
}

//nolint:gocognit // one order write: idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction
func (c *UseCase) Place(
	ctx context.Context,
	userID uuid.UUID,
	in domain.NewOrder,
	idempotencyKey string,
) (*domain.Order, error) {
	existing, err := c.repo.GetByUserIDAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		items, itemErr := c.repo.ListItemsByOrderID(ctx, existing.ID)
		if itemErr != nil {
			return nil, itemErr
		}
		existing.Items = items
		return existing, nil
	}

	order := &domain.Order{
		UserID:          userID,
		IdempotencyKey:  idempotencyKey,
		Status:          domain.StatusAwaitingPayment,
		CouponCode:      in.CouponCode,
		ShippingAddress: in.ShippingAddress,
		BillingAddress:  in.BillingAddress,
		Notes:           in.Notes,
	}

	var orderItems []domain.Item

	err = c.tx.Run(ctx, func(txCtx context.Context) error {
		if txErr := c.locker.Lock(txCtx, userID); txErr != nil {
			if errors.Is(txErr, apperror.ErrNotFound) {
				return apperror.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := c.carts.Snapshot(txCtx, userID)
		if txErr != nil {
			return txErr
		}
		if len(snapshot.Items) == 0 {
			return apperror.ErrCartEmpty
		}

		reservations := make(map[uuid.UUID]int, len(snapshot.Items))
		orderItems = make([]domain.Item, len(snapshot.Items))
		subtotal := money.New(0, snapshot.Items[0].Price.Currency)
		for _, item := range snapshot.Items {
			if item.Status != productStatusPublished {
				return fmt.Errorf("%w: product %s is not available", apperror.ErrBadRequest, item.Name)
			}
			sum, addErr := subtotal.Add(item.Price.MulQty(item.Quantity))
			if addErr != nil {
				return fmt.Errorf("%w: cart contains mixed currencies: %w", apperror.ErrBadRequest, addErr)
			}
			subtotal = sum
			reservations[item.ProductID] = item.Quantity
		}

		order.Subtotal = subtotal
		order.Total = subtotal
		order.Discount = money.New(0, subtotal.Currency)
		if txErr := c.repo.Create(txCtx, order); txErr != nil {
			return txErr
		}

		if txErr := c.reserver.Reserve(txCtx, reservations); txErr != nil {
			return fmt.Errorf("reserving stock: %w", txErr)
		}

		for i, item := range snapshot.Items {
			orderItems[i] = domain.Item{
				OrderID:     order.ID,
				ProductID:   item.ProductID,
				ProductName: item.Name,
				Price:       item.Price,
				Quantity:    item.Quantity,
				Subtotal:    item.Price.MulQty(item.Quantity),
			}
		}
		if txErr := c.repo.CreateItems(txCtx, orderItems); txErr != nil {
			return txErr
		}

		if c.coupons != nil && in.CouponCode != nil && *in.CouponCode != "" {
			discount, txErr := c.coupons.Reserve(txCtx, *in.CouponCode, userID, order.ID, subtotal.Amount)
			if txErr != nil {
				return txErr
			}
			order.Discount = money.New(discount, subtotal.Currency)
			order.Total = money.New(max(subtotal.Amount-discount, 0), subtotal.Currency)
			if txErr := c.repo.UpdateTotals(txCtx, order.ID, order.Discount.Amount, order.Total.Amount); txErr != nil {
				return txErr
			}
		}

		return c.clearer.Clear(txCtx, userID)
	})
	if err != nil {
		return nil, err
	}

	order.Items = orderItems

	if order.Total.Amount == 0 {
		if freeErr := c.finalizeFreeOrder(ctx, order); freeErr != nil {
			c.logger.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
				slog.String("order_id", order.ID.String()), slog.String("error", freeErr.Error()))
		}
	}

	if c.notifications != nil {
		if err := c.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			c.logger.WarnContext(ctx, "failed to enqueue order placed notification", slog.String("error", err.Error()))
		}
	}

	return order, nil
}

func (c *UseCase) finalizeFreeOrder(ctx context.Context, order *domain.Order) error {
	return c.tx.Run(ctx, func(txCtx context.Context) error {
		if err := c.transition.Apply(txCtx, order.ID, domain.PaidTransition); err != nil {
			return err
		}
		deductions := make(map[uuid.UUID]int, len(order.Items))
		for _, item := range order.Items {
			deductions[item.ProductID] = item.Quantity
		}
		return c.deductor.Deduct(txCtx, deductions)
	})
}
