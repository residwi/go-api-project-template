package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/contract"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

const housekeepingBatchLimit = 20

const productStatusPublished = "published"

type Service struct {
	repo          Repository
	tx            database.TxRunner
	cart          CartProvider
	inventory     InventoryReserver
	payment       PaymentInitiator
	paymentCancel PaymentJobCanceller
	coupons       CouponReserver
	notifications NotificationEnqueuer
	logger        *slog.Logger
}

func NewService(
	repo Repository,
	tx database.TxRunner,
	cart CartProvider,
	inventory InventoryReserver,
	payment PaymentInitiator,
	paymentCancel PaymentJobCanceller,
	coupons CouponReserver,
	notifications NotificationEnqueuer,
	log *slog.Logger,
) *Service {
	return &Service{
		repo:          repo,
		tx:            tx,
		cart:          cart,
		inventory:     inventory,
		payment:       payment,
		paymentCancel: paymentCancel,
		coupons:       coupons,
		notifications: notifications,
		logger:        log,
	}
}

//nolint:gocognit,funlen // checkout orchestrates idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction
func (s *Service) PlaceOrder(
	ctx context.Context,
	userID uuid.UUID,
	p PlaceParams,
	idempotencyKey string,
) (*PlaceResult, error) {
	existing, err := s.repo.GetByUserIDAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		items, itemErr := s.repo.ListItemsByOrderID(ctx, existing.ID)
		if itemErr != nil {
			return nil, itemErr
		}
		existing.Items = items
		return &PlaceResult{Order: existing}, nil
	}

	order := &Order{
		UserID:          userID,
		IdempotencyKey:  idempotencyKey,
		Status:          StatusAwaitingPayment,
		CouponCode:      p.CouponCode,
		ShippingAddress: p.ShippingAddress,
		BillingAddress:  p.BillingAddress,
		Notes:           p.Notes,
	}

	var orderItems []Item

	err = s.tx.Run(ctx, func(txCtx context.Context) error {
		// Read the cart INSIDE the transaction, after the lock: a second concurrent
		// checkout then blocks, and reads the emptied cart instead of replaying the
		// same items. Idempotency-Key only dedupes retries of one request.
		if txErr := s.cart.LockCart(txCtx, userID); txErr != nil {
			if errors.Is(txErr, apperror.ErrNotFound) {
				return apperror.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := s.cart.GetSnapshot(txCtx, userID)
		if txErr != nil {
			return txErr
		}
		if len(snapshot.Items) == 0 {
			return apperror.ErrCartEmpty
		}

		// A cart is keyed by product, so this cannot receive a duplicate ProductID.
		reservations := make(map[uuid.UUID]int, len(snapshot.Items))
		orderItems = make([]Item, len(snapshot.Items))
		// Seeded from item 0 so that item runs the loop's availability check too.
		subtotal := money.New(0, snapshot.Items[0].Price.Currency)
		for _, item := range snapshot.Items {
			if item.Status != productStatusPublished {
				return fmt.Errorf("%w: product %s is not available", apperror.ErrBadRequest, item.Name)
			}
			sum, addErr := subtotal.Add(item.Price.MulQty(item.Quantity))
			if addErr != nil {
				// Both sentinels: ErrBadRequest gives the 400 (mixed currencies are user
				// input), ErrCurrencyMismatch names the cause.
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

		if txErr := s.inventory.ReserveBatch(txCtx, reservations); txErr != nil {
			return fmt.Errorf("reserving stock: %w", txErr)
		}

		// A second pass because items need order.ID, which only exists after Create.
		for i, item := range snapshot.Items {
			orderItems[i] = Item{
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

		if s.coupons != nil && p.CouponCode != nil && *p.CouponCode != "" {
			discount, txErr := s.coupons.Reserve(txCtx, *p.CouponCode, userID, order.ID, subtotal.Amount)
			if txErr != nil {
				return txErr
			}
			// max(..., 0) is this caller's policy, not money's.
			order.Discount = money.New(discount, subtotal.Currency)
			order.Total = money.New(max(subtotal.Amount-discount, 0), subtotal.Currency)
			// The row was inserted pre-discount; payment finalization verifies against
			// what is stored here.
			if txErr := s.repo.UpdateTotals(txCtx, order.ID, order.Discount.Amount, order.Total.Amount); txErr != nil {
				return txErr
			}
		}

		return s.cart.Clear(txCtx, userID)
	})
	if err != nil {
		return nil, err
	}

	order.Items = orderItems

	if order.Total.Amount > 0 {
		// Not fatal: the order stays awaiting_payment for the webhook, a retry, or
		// the expiry sweep.
		if _, payErr := s.payment.InitiatePayment(ctx, paymentcontract.ChargeRequest{
			OrderID:         order.ID,
			Amount:          order.Total,
			PaymentMethodID: p.PaymentMethodID,
		}); payErr != nil {
			s.logger.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				slog.String("order_id", order.ID.String()), slog.String("error", payErr.Error()))
		}
	} else if freeErr := s.finalizeFreeOrder(ctx, order); freeErr != nil {
		s.logger.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
			slog.String("order_id", order.ID.String()), slog.String("error", freeErr.Error()))
	}

	if s.notifications != nil {
		if err := s.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			s.logger.WarnContext(ctx, "failed to enqueue order placed notification", slog.String("error", err.Error()))
		}
	}

	return &PlaceResult{Order: order}, nil
}

func (s *Service) RetryPayment(
	ctx context.Context,
	userID, orderID uuid.UUID,
	paymentMethodID string,
) (*paymentcontract.ChargeResult, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}
	if order.Status != StatusAwaitingPayment {
		return nil, apperror.ErrOrderNotPayable
	}

	result, err := s.payment.InitiatePayment(ctx, paymentcontract.ChargeRequest{
		OrderID:         order.ID,
		Amount:          order.Total,
		PaymentMethodID: paymentMethodID,
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *Service) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if order.UserID != userID {
		return apperror.ErrNotFound
	}

	if order.Status == StatusPaymentProcessing {
		return apperror.ErrOrderCharging
	}

	if err := s.cancelWithReversal(ctx, order); err != nil {
		return err
	}

	if s.paymentCancel != nil {
		if err := s.paymentCancel.CancelJobsByOrderID(ctx, orderID); err != nil {
			s.logger.WarnContext(
				ctx,
				"failed to cancel payment jobs",
				slog.String("order_id", orderID.String()),
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

// CancelUnpaid is system-initiated (the payment webhook), so unlike
// CancelOrder it runs no ownership check. The CancelledTransition CAS still
// rejects an already-paid order as a wrapped apperror.ErrBadRequest. Named
// for payment.OrderUpdater's intent, which this satisfies directly.
func (s *Service) CancelUnpaid(ctx context.Context, orderID uuid.UUID) error {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	return s.cancelWithReversal(ctx, order)
}

// The Mark* methods below are named for the capability their callers ask for,
// so payment.OrderUpdater and shipping.OrderPorts are satisfied without an
// adapter. Each maps to exactly one named Transition, so the allowed-from set
// stays declared once, in transition.go.

func (s *Service) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, PaymentProcessingTransition)
}

func (s *Service) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, AwaitingPaymentTransition)
}

func (s *Service) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, PaidTransition)
}

func (s *Service) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, FulfillmentFailedAfterChargeTransition)
}

func (s *Service) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, FulfillmentFailedCompensatingTransition)
}

func (s *Service) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, RefundTransition)
}

func (s *Service) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, ShippedTransition)
}

func (s *Service) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return s.Apply(ctx, orderID, DeliveredTransition)
}

// ExpireStale is the payment runner's per-tick Sweeper hook. Each order gets
// its own transaction, so one failure is logged and the sweep continues.
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

// RecoverStaleProcessing un-strands orders left in payment_processing by a
// worker that died mid-charge, handing them back to the retry/expiry path.
// The CAS only matches payment_processing, so a concurrent recovery no-ops.
func (s *Service) RecoverStaleProcessing(ctx context.Context) error {
	orders, err := s.repo.GetStaleProcessingOrders(ctx, contract.StaleProcessingThreshold, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting stale processing orders: %w", err)
	}
	for _, o := range orders {
		if err := s.repo.Apply(ctx, o.ID, AwaitingPaymentTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				continue // already moved on by another worker
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

func (s *Service) GetByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
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

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]Order, error) {
	return s.repo.ListByUser(ctx, userID, cursor)
}

func (s *Service) AdminListAll(ctx context.Context, params AdminListParams) ([]Order, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*Order, error) {
	return s.repo.GetByID(ctx, orderID)
}

func (s *Service) AdminGetByID(ctx context.Context, orderID uuid.UUID) (*Order, error) {
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

func (s *Service) AdminUpdateStatus(ctx context.Context, orderID uuid.UUID, toStatus Status) error {
	// A bare status write here would mark an order paid without deducting stock,
	// or refunded without restocking it or returning money at the gateway. Any
	// status with those side effects belongs to the flow that owns them.
	// Only the side-effect-free fulfillment markers may be set directly.
	switch toStatus {
	case StatusPaid, StatusPaymentProcessing, StatusCancelled, StatusExpired, StatusRefunded, StatusFulfillmentFailed:
		// Reachable from paid, i.e. money captured and stock deducted: a bare write
		// here would strand both. It belongs to the compensating refund flow.
		return fmt.Errorf(
			"%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update",
			apperror.ErrBadRequest,
			toStatus,
		)
	case StatusAwaitingPayment, StatusProcessing, StatusShipped, StatusDelivered:
		// None of these reverse inventory or payment, so they may be set directly.
	}

	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !CanTransition(order.Status, toStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", apperror.ErrBadRequest, order.Status, toStatus)
	}

	return s.repo.UpdateStatus(ctx, orderID, order.Status, toStatus)
}

// Apply is the single entry point for a status change: a compare-and-set that
// returns apperror.ErrConflict when the current status is not in t.From.
// Callers name a transition from transition.go, never an ad-hoc status list.
func (s *Service) Apply(ctx context.Context, orderID uuid.UUID, t Transition) error {
	return s.repo.Apply(ctx, orderID, t)
}

func (s *Service) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error) {
	return s.repo.ListItemsByOrderID(ctx, orderID)
}

// GetSnapshot backs payment.OrderGetter: everything payment needs to decide a
// charge or refund outcome, without payment importing order.
func (s *Service) GetSnapshot(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	o, err := s.GetOrderByID(ctx, orderID)
	if err != nil {
		return contract.Order{}, err
	}

	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}

	return contract.Order{
		Total:         o.Total,
		Status:        string(o.Status),
		CouponCode:    couponCode,
		StockDeducted: o.StockDeducted,
		StockReversed: o.StockReversed,
		Dispatched:    o.Dispatched(),
	}, nil
}

// GetInfo backs shipping's per-slice ownership checks (query.OrderProvider,
// create.OrderPort), which need only who owns the order and its current status.
func (s *Service) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	o, err := s.GetOrderByID(ctx, orderID)
	if err != nil {
		return contract.Order{}, err
	}
	return contract.Order{ID: o.ID, UserID: o.UserID, Status: string(o.Status)}, nil
}

// ListItemQuantities backs payment.OrderItemsGetter. A paid order has one
// order line per product, so this cannot collide two items into the same key.
func (s *Service) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	items, err := s.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]int, len(items))
	for _, item := range items {
		out[item.ProductID] = item.Quantity
	}
	return out, nil
}

// DeliveredPurchaseParams names its fields because all three ids are uuid.UUID:
// a positional swap would compile and answer about the wrong purchase. This is
// order's own repository parameter, not a seam type: review crosses the seam
// with three plain uuid.UUID arguments instead.
type DeliveredPurchaseParams struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

// HasDeliveredOrder backs review.PurchaseVerifier, so review confirms a
// purchase through this module instead of querying the orders schema.
func (s *Service) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return s.repo.HasDeliveredOrder(ctx, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	})
}

// SetPaymentDeps breaks the order/payment construction cycle.
func (s *Service) SetPaymentDeps(payment PaymentInitiator, paymentCancel PaymentJobCanceller) {
	s.payment = payment
	s.paymentCancel = paymentCancel
}

// finalizeFreeOrder settles a coupon-covered order that has no payment at all.
// Without it the order would sit in awaiting_payment until the expiry sweep
// cancelled it, so a legitimately free order could never ship.
func (s *Service) finalizeFreeOrder(ctx context.Context, order *Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Apply(txCtx, order.ID, PaidTransition); err != nil {
			return err
		}
		// One order line per product by construction (see PlaceOrder), so no
		// ProductID can collide here.
		deductions := make(map[uuid.UUID]int, len(order.Items))
		for _, item := range order.Items {
			deductions[item.ProductID] = item.Quantity
		}
		return s.inventory.DeductBatch(txCtx, deductions)
	})
}

// cancelWithReversal is the single cancel path, shared by the user-facing
// CancelOrder and the system-facing CancelUnpaid. One transaction: a failed
// reversal rolls the cancel back, so no order is cancelled with stock held.
//
//nolint:gocognit // the single cancel path: guarded status CAS, conditional stock reversal (release vs restock vs skip), and best-effort coupon release
func (s *Service) cancelWithReversal(ctx context.Context, order *Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if txErr := s.repo.Apply(txCtx, order.ID, CancelledTransition); txErr != nil {
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

func (s *Service) expireOne(ctx context.Context, o Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Apply(txCtx, o.ID, ExpiredTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				return nil // another worker already moved it out of awaiting_payment
			}
			return err
		}
		return s.releaseOrderHolds(txCtx, o)
	})
}

// releaseOrderHolds serves the expire path only, which sees awaiting_payment
// orders: their stock is reserved, never deducted, so a release is always right.
func (s *Service) releaseOrderHolds(ctx context.Context, o Order) error {
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

// stockStateFor keeps the contract.StockState enum out of the persisted Order:
// StockDeducted stays a plain bool column, and only this seam translates it.
func stockStateFor(deducted bool) inventorycontract.StockState {
	if deducted {
		return inventorycontract.Deducted
	}
	return inventorycontract.Reserved
}
