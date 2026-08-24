package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

const (
	productStatusPublished = "published"
	housekeepingBatchLimit = 20
)

type Deps struct {
	Repo   Repository
	Tx     database.TxRunner
	Logger *slog.Logger

	Cart          Cart
	Inventory     Inventory
	Coupons       CouponReserver
	Notifications Notifications
}

type Service struct {
	repo   Repository
	tx     database.TxRunner
	logger *slog.Logger

	cart          Cart
	inventory     Inventory
	coupons       CouponReserver
	notifications Notifications
}

func New(d Deps) *Service {
	return &Service{
		repo:          d.Repo,
		tx:            d.Tx,
		logger:        d.Logger,
		cart:          d.Cart,
		inventory:     d.Inventory,
		coupons:       d.Coupons,
		notifications: d.Notifications,
	}
}

// Place reports whether it created the order. A repeated idempotency key
// returns the stored order with created=false, and that flag is the only signal
// the caller gets: the returned order looks the same either way, so anything a
// replay must not repeat -- charging a card, above all -- has to branch on it
// rather than infer a replay from order status.
//
//nolint:gocognit // one order write: idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction
func (s *Service) Place(
	ctx context.Context,
	userID uuid.UUID,
	in domain.NewOrder,
	idempotencyKey string,
) (*domain.Order, bool, error) {
	existing, err := s.repo.GetByUserIDAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, false, err
	}
	if existing != nil {
		items, itemErr := s.repo.ListItemsByOrderID(ctx, existing.ID)
		if itemErr != nil {
			return nil, false, itemErr
		}
		existing.Items = items
		return existing, false, nil
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

	err = s.tx.Run(ctx, func(txCtx context.Context) error {
		if txErr := s.cart.Lock(txCtx, userID); txErr != nil {
			if errors.Is(txErr, apperror.ErrNotFound) {
				return apperror.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := s.cart.Snapshot(txCtx, userID)
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
		if txErr := s.repo.Create(txCtx, order); txErr != nil {
			return txErr
		}

		if txErr := s.inventory.Reserve(txCtx, reservations); txErr != nil {
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
		if txErr := s.repo.CreateItems(txCtx, orderItems); txErr != nil {
			return txErr
		}

		if s.coupons != nil && in.CouponCode != nil && *in.CouponCode != "" {
			discount, txErr := s.coupons.Reserve(txCtx, *in.CouponCode, userID, order.ID, subtotal.Amount)
			if txErr != nil {
				return txErr
			}
			order.Discount = money.New(discount, subtotal.Currency)
			order.Total = money.New(max(subtotal.Amount-discount, 0), subtotal.Currency)
			if txErr := s.repo.UpdateTotals(txCtx, order.ID, order.Discount.Amount, order.Total.Amount); txErr != nil {
				return txErr
			}
		}

		return s.cart.Clear(txCtx, userID)
	})
	if err != nil {
		return nil, false, err
	}

	order.Items = orderItems

	if order.Total.Amount == 0 {
		if freeErr := s.finalizeFreeOrder(ctx, order); freeErr != nil {
			s.logger.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
				slog.String("order_id", order.ID.String()), slog.String("error", freeErr.Error()))
		}
	}

	if s.notifications != nil {
		if err := s.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			s.logger.WarnContext(ctx, "failed to enqueue order placed notification", slog.String("error", err.Error()))
		}
	}

	return order, true, nil
}

func (s *Service) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Order, error) {
	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) GetForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Order, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}

	items, err := s.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = items

	return order, nil
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Order, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) Get(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = items

	return order, nil
}

// Snapshot is the one projection every consuming module reads. It replaces the
// pair this module used to expose -- GetSnapshot, which filled every field, and
// GetInfo, which filled only ID, UserID and Status off the identical read. The
// full value satisfies every consumer, so the sparse one was a second way to
// get the same row wrong.
func (s *Service) Snapshot(ctx context.Context, orderID uuid.UUID) (Snapshot, error) {
	o, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return Snapshot{}, err
	}

	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}

	return Snapshot{
		ID:            o.ID,
		UserID:        o.UserID,
		Total:         o.Total,
		Status:        string(o.Status),
		CouponCode:    couponCode,
		StockDeducted: o.StockDeducted,
		StockReversed: o.StockReversed,
		Dispatched:    o.Dispatched(),
	}, nil
}

func (s *Service) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	items, err := s.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]int, len(items))
	for _, item := range items {
		out[item.ProductID] = item.Quantity
	}
	return out, nil
}

func (s *Service) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return s.repo.HasDeliveredOrder(ctx, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	})
}

func (s *Service) CancelByUser(ctx context.Context, userID, orderID uuid.UUID) error {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if order.UserID != userID {
		return apperror.ErrNotFound
	}

	if order.Status == domain.StatusPaymentProcessing {
		return apperror.ErrOrderCharging
	}

	return s.cancelWithReversal(ctx, order)
}

func (s *Service) CancelUnpaid(ctx context.Context, orderID uuid.UUID) error {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	return s.cancelWithReversal(ctx, order)
}

// ChangeStatus is the admin's direct status write. It refuses every status a
// saga owns, so nothing bypasses the inventory or money reversal that status
// carries; the rest go through the dynamic compare-and-set.
func (s *Service) ChangeStatus(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error {
	switch toStatus {
	case domain.StatusPaid, domain.StatusPaymentProcessing, domain.StatusCancelled,
		domain.StatusExpired, domain.StatusRefunded, domain.StatusFulfillmentFailed:
		return fmt.Errorf(
			"%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update",
			apperror.ErrBadRequest,
			toStatus,
		)
	case domain.StatusAwaitingPayment, domain.StatusProcessing, domain.StatusShipped, domain.StatusDelivered:
	}

	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !domain.CanTransition(order.Status, toStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", apperror.ErrBadRequest, order.Status, toStatus)
	}

	return s.UpdateStatus(ctx, orderID, order.Status, toStatus)
}

func (s *Service) ExpireStale(ctx context.Context) error {
	orders, err := s.repo.GetExpiredOrders(ctx, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting expired orders: %w", err)
	}
	for _, o := range orders {
		if err := s.expireOne(ctx, o); err != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to expire order",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func (s *Service) RecoverStale(ctx context.Context) error {
	orders, err := s.repo.GetStaleProcessingOrders(ctx, StaleProcessingThreshold, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting stale processing orders: %w", err)
	}
	for _, o := range orders {
		if err := s.Apply(ctx, o.ID, domain.AwaitingPaymentTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				continue
			}
			s.logger.ErrorContext(
				ctx,
				"failed to recover stale processing order",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

// Apply is the one guarded status write, per AGENTS.md rule 14: every caller
// names a domain.Transition value from domain/transition.go rather than an
// ad-hoc from/to pair.
func (s *Service) Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error {
	return s.repo.Apply(ctx, orderID, t)
}

func (s *Service) UpdateStatus(ctx context.Context, orderID uuid.UUID, from, to domain.Status) error {
	return s.repo.UpdateStatus(ctx, orderID, from, to)
}

func (s *Service) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.PaymentProcessingTransition)
}

// BeginPaymentAttempt returns apperror.ErrConflict unless the order was
// awaiting payment, so a caller may charge only once this returns nil.
func (s *Service) BeginPaymentAttempt(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.PaymentAttemptTransition)
}

func (s *Service) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.AwaitingPaymentTransition)
}

func (s *Service) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.PaidTransition)
}

func (s *Service) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.FulfillmentFailedAfterChargeTransition)
}

func (s *Service) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.FulfillmentFailedCompensatingTransition)
}

func (s *Service) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.RefundTransition)
}

func (s *Service) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.ShippedTransition)
}

func (s *Service) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, domain.DeliveredTransition)
}

func (s *Service) finalizeFreeOrder(ctx context.Context, order *domain.Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.Apply(txCtx, order.ID, domain.PaidTransition); err != nil {
			return err
		}
		deductions := make(map[uuid.UUID]int, len(order.Items))
		for _, item := range order.Items {
			deductions[item.ProductID] = item.Quantity
		}
		return s.inventory.Deduct(txCtx, deductions)
	})
}

//nolint:gocognit // the single cancel path: guarded status CAS, conditional stock reversal (release vs restock vs skip), and best-effort coupon release
func (s *Service) cancelWithReversal(ctx context.Context, order *domain.Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if txErr := s.Apply(txCtx, order.ID, domain.CancelledTransition); txErr != nil {
			if errors.Is(txErr, apperror.ErrConflict) {
				return fmt.Errorf("%w: cannot cancel order in status %s", apperror.ErrBadRequest, order.Status)
			}
			return txErr
		}

		items, txErr := s.repo.ListItemsByOrderID(txCtx, order.ID)
		if txErr != nil {
			return txErr
		}
		if len(items) > 0 && !order.StockReversed {
			releases := make(map[uuid.UUID]int, len(items))
			for _, item := range items {
				releases[item.ProductID] = item.Quantity
			}
			releaseErr := s.inventory.Restore(txCtx, releases, stockStateFor(order.StockDeducted))
			if releaseErr != nil {
				return fmt.Errorf("restoring inventory on cancel: %w", releaseErr)
			}
		}

		if s.coupons != nil && order.CouponCode != nil && *order.CouponCode != "" {
			if releaseErr := s.coupons.Release(txCtx, order.ID); releaseErr != nil {
				s.logger.WarnContext(
					txCtx,
					"failed to release coupon on cancel",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		return nil
	})
}

func (s *Service) expireOne(ctx context.Context, o domain.Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.Apply(txCtx, o.ID, domain.ExpiredTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				return nil
			}
			return err
		}
		return s.releaseOrderHolds(txCtx, o)
	})
}

func (s *Service) releaseOrderHolds(ctx context.Context, o domain.Order) error {
	items, err := s.repo.ListItemsByOrderID(ctx, o.ID)
	if err != nil {
		return err
	}
	if len(items) > 0 && !o.StockReversed {
		releases := make(map[uuid.UUID]int, len(items))
		for _, item := range items {
			releases[item.ProductID] = item.Quantity
		}
		if err := s.inventory.Restore(ctx, releases, stockStateFor(o.StockDeducted)); err != nil {
			return fmt.Errorf("restoring inventory on expire: %w", err)
		}
	}

	if s.coupons != nil && o.CouponCode != nil && *o.CouponCode != "" {
		if err := s.coupons.Release(ctx, o.ID); err != nil {
			s.logger.WarnContext(
				ctx,
				"failed to release coupon on expire",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func stockStateFor(deducted bool) inventory.StockState {
	if deducted {
		return inventory.Deducted
	}
	return inventory.Reserved
}
