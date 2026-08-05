package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// staleProcessingThreshold is how long an order may sit in payment_processing
// before the sweep treats its charge attempt as dead and reverts it to
// awaiting_payment. It must comfortably exceed a charge job's lease.
const staleProcessingThreshold = 15 * time.Minute

// housekeepingBatchLimit bounds how many orders each sweep pass touches.
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
		// Lock the cart row, then read its contents INSIDE the transaction. This
		// serializes concurrent checkouts of the same cart: a second checkout
		// blocks on the lock until the first commits (clearing the cart), then
		// reads an empty cart and aborts — instead of replaying the same items
		// into a second order. The Idempotency-Key only dedupes retries of one
		// request, not two distinct concurrent checkouts.
		if txErr := s.cart.LockCart(txCtx, userID); txErr != nil {
			if errors.Is(txErr, apperror.ErrNotFound) {
				return apperror.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := s.cart.GetCart(txCtx, userID)
		if txErr != nil {
			return txErr
		}
		if len(snapshot.Items) == 0 {
			return apperror.ErrCartEmpty
		}

		reservations := make([]InventoryItem, len(snapshot.Items))
		orderItems = make([]Item, len(snapshot.Items))
		// Seed the running subtotal with a zero denominated in the first item's
		// currency, so that item 0 goes through the loop -- and its availability
		// check -- exactly like every other item. Money.Add then enforces the
		// single-currency rule that used to be a hand-rolled comparison: summing
		// across currencies would produce a meaningless total, and an arbitrary
		// order currency. The empty-cart guard above makes Items[0] safe.
		subtotal := money.New(0, snapshot.Items[0].Price.Currency)
		for i, item := range snapshot.Items {
			if item.Status != productStatusPublished {
				return fmt.Errorf("%w: product %s is not available", apperror.ErrBadRequest, item.Name)
			}
			sum, addErr := subtotal.Add(item.Price.MulQty(item.Quantity))
			if addErr != nil {
				// Both sentinels: ErrBadRequest keeps the 400 the old hand-rolled
				// check produced (a mixed-currency cart is user input, not a server
				// fault), ErrCurrencyMismatch names the actual cause.
				return fmt.Errorf("%w: cart contains mixed currencies: %w", apperror.ErrBadRequest, addErr)
			}
			subtotal = sum
			reservations[i] = InventoryItem{ProductID: item.ProductID, Quantity: item.Quantity}
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

		// A second pass, not a merge into the one above: the items need order.ID,
		// which only exists after repo.Create. Price carries the cart item's own
		// currency, which the fold above has already proved equal to the order's.
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
			// A discount is denominated in the order's currency by construction, so
			// the clamp stays plain arithmetic on the amounts: max(..., 0) is the
			// policy (an over-large coupon does not produce a negative charge), and
			// that policy is the caller's, not the money package's.
			order.Discount = money.New(discount, subtotal.Currency)
			order.Total = money.New(max(subtotal.Amount-discount, 0), subtotal.Currency)
			// The order row was inserted with the pre-discount total; persist the
			// discounted amounts so the DB matches what we charge and what payment
			// finalization verifies against.
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
		// Discard the result: the order stays awaiting_payment and the gateway
		// webhook (or the charge job) drives it to paid. A failure here is logged,
		// not fatal — the order can be retried or will expire.
		if _, payErr := s.payment.InitiatePayment(ctx, InitiatePaymentParams{
			OrderID:         order.ID,
			Amount:          order.Total,
			PaymentMethodID: p.PaymentMethodID,
		}); payErr != nil {
			s.logger.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				slog.Any("order_id", order.ID), slog.Any("error", payErr))
		}
	} else if freeErr := s.finalizeFreeOrder(ctx, order); freeErr != nil {
		// A fully-discounted order has nothing to charge; if it can't be finalized
		// now it stays in awaiting_payment and the expiry sweep cancels it.
		s.logger.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
			slog.Any("order_id", order.ID), slog.Any("error", freeErr))
	}

	if s.notifications != nil {
		if err := s.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			s.logger.WarnContext(ctx, "failed to enqueue order placed notification", slog.Any("error", err))
		}
	}

	return &PlaceResult{Order: order}, nil
}

func (s *Service) RetryPayment(
	ctx context.Context,
	userID, orderID uuid.UUID,
	paymentMethodID string,
) (*PaymentResult, error) {
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

	result, err := s.payment.InitiatePayment(ctx, InitiatePaymentParams{
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
				slog.Any("order_id", orderID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

// CancelUnpaidByID cancels an order whose payment terminally failed and releases
// its holds. It is system-initiated (used by the payment webhook), so unlike
// CancelOrder it performs no ownership check; the CancelledTransition CAS still
// rejects an already-paid or terminal order, surfaced as a wrapped ErrBadRequest.
func (s *Service) CancelUnpaidByID(ctx context.Context, orderID uuid.UUID) error {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	return s.cancelWithReversal(ctx, order)
}

// ExpireStale expires awaiting_payment orders whose payment window has lapsed,
// releasing the stock and coupon each reserved — the time-triggered sibling of
// CancelOrder. It is the per-tick housekeeping the payment job runner invokes
// via its Sweeper hook. Each order is handled in its own transaction so a
// per-order failure is logged and the sweep continues.
func (s *Service) ExpireStale(ctx context.Context) error {
	orders, err := s.repo.GetExpiredOrders(ctx, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting expired orders: %w", err)
	}
	for _, o := range orders {
		if err := s.expireOne(ctx, o); err != nil {
			s.logger.ErrorContext(ctx, "failed to expire order", slog.Any("order_id", o.ID), slog.Any("error", err))
		}
	}
	return nil
}

// RecoverStaleProcessing reverts orders stuck in payment_processing — e.g. a
// worker that died after claiming a charge but before the order moved on — back
// to awaiting_payment, so the normal retry/expiry path takes over instead of the
// order being stranded forever. It is the payment runner's per-tick housekeeping
// alongside ExpireStale. The AwaitingPaymentTransition CAS only matches orders
// still in payment_processing, so a concurrent recovery is a harmless no-op.
func (s *Service) RecoverStaleProcessing(ctx context.Context) error {
	orders, err := s.repo.GetStaleProcessingOrders(ctx, staleProcessingThreshold, housekeepingBatchLimit)
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
				slog.Any("order_id", o.ID),
				slog.Any("error", err),
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

// GetOrderByID returns the order WITHOUT its line items. Adapters that only need
// order-level fields (payment, shipping) use this to avoid the extra order_items
// query that GetByID/AdminGetByID issue.
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
	// Statuses that deduct, release, or restock stock — or return captured money —
	// must go through the flow that owns those side effects (the payment charge
	// job for paid/payment_processing, user/admin cancel for cancelled, the
	// expiry sweep for expired, the payment refund job for refunded). A bare
	// status write here would, for example, mark an order paid without deducting
	// stock, or refunded without restocking it or returning money at the gateway.
	// Only the side-effect-free fulfillment markers may be set directly.
	switch toStatus {
	case StatusPaid, StatusPaymentProcessing, StatusCancelled, StatusExpired, StatusRefunded, StatusFulfillmentFailed:
		// fulfillment_failed is reachable from paid (FulfillmentFailedCompensatingTransition),
		// i.e. from an order with money captured and stock deducted. A bare status
		// write here would strand both — no refund, no restock — so it must go
		// through the payment/refund compensating flow, not a direct admin update.
		return fmt.Errorf(
			"%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update",
			apperror.ErrBadRequest,
			toStatus,
		)
	case StatusAwaitingPayment, StatusProcessing, StatusShipped, StatusDelivered:
		// Side-effect-free fulfillment markers — allowed to be set directly below
		// (subject to CanTransition); none of these reverse inventory or payment.
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

// Apply performs the guarded status transition t (a compare-and-set): it moves
// the order to t.To only if its current status is one of t.From, returning
// apperror.ErrConflict if nothing matched. It is the single entry point the
// cross-feature bootstrap adapters call — each names its transition in
// transition.go rather than passing ad-hoc from/to status lists.
func (s *Service) Apply(ctx context.Context, orderID uuid.UUID, t Transition) error {
	return s.repo.Apply(ctx, orderID, t)
}

// ListItemsByOrderID is used by payment service adapter.
func (s *Service) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error) {
	return s.repo.ListItemsByOrderID(ctx, orderID)
}

// DeliveredPurchaseParams names each id so a caller cannot transpose them; all
// three are uuid.UUID and a positional swap would compile and silently return
// the wrong verdict on whether this customer may review this product.
type DeliveredPurchaseParams struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

// HasDeliveredOrder reports whether the given order is delivered, belongs to the
// user, and contains the product. A bootstrap adapter maps this onto
// review.PurchaseVerifier, letting review confirm a specific purchase through
// the order module rather than querying the orders schema directly from the
// bootstrap layer.
func (s *Service) HasDeliveredOrder(ctx context.Context, p DeliveredPurchaseParams) (bool, error) {
	return s.repo.HasDeliveredOrder(ctx, p)
}

// SetPaymentDeps sets payment-related dependencies after construction.
// This breaks the circular dependency between order and payment services.
func (s *Service) SetPaymentDeps(payment PaymentInitiator, paymentCancel PaymentJobCanceller) {
	s.payment = payment
	s.paymentCancel = paymentCancel
}

// finalizeFreeOrder settles a zero-total order (a coupon covered the full
// subtotal) that has no payment: it marks the order paid and deducts the
// reserved stock in one transaction, mirroring FinalizePaymentSuccess for a
// charged order. Apply(PaidTransition) also sets the order's stock_deducted flag
// atomically. Without this the order would sit in awaiting_payment and be
// cancelled by the expiry sweep, so a legitimately free order could never ship.
func (s *Service) finalizeFreeOrder(ctx context.Context, order *Order) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Apply(txCtx, order.ID, PaidTransition); err != nil {
			return err
		}
		deductions := make([]InventoryItem, len(order.Items))
		for i, item := range order.Items {
			deductions[i] = InventoryItem{ProductID: item.ProductID, Quantity: item.Quantity}
		}
		return s.inventory.DeductBatch(txCtx, deductions)
	})
}

// cancelWithReversal moves an order to cancelled and reverses its inventory hold
// and coupon in one transaction — the single cancel path shared by the
// user-facing CancelOrder and the system-facing CancelUnpaidByID. Routing the
// status change through CancelledTransition keeps the allowed-from set in one
// place, and the reversal honors the order's persisted stock state (release vs
// restock vs skip-if-already-reversed). A reversal failure rolls back the cancel
// too, so an order is never committed cancelled while its stock stays held.
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
			releases := make([]InventoryItem, len(items))
			for i, item := range items {
				releases[i] = InventoryItem{ProductID: item.ProductID, Quantity: item.Quantity}
			}
			if releaseErr := s.inventory.Restore(txCtx, releases, order.StockDeducted); releaseErr != nil {
				return fmt.Errorf("restoring inventory on cancel: %w", releaseErr)
			}
		}

		if s.coupons != nil && order.CouponCode != nil && *order.CouponCode != "" {
			if releaseErr := s.coupons.Release(txCtx, order.ID); releaseErr != nil {
				s.logger.WarnContext(txCtx, "failed to release coupon on cancel", slog.Any("error", releaseErr))
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

// releaseOrderHolds returns an order's reserved stock and coupon usage. Shared
// by the expire path; expiry only applies to awaiting_payment orders, whose
// stock is reserved-only and not yet reversed, so this releases the reservation.
func (s *Service) releaseOrderHolds(ctx context.Context, o Order) error {
	items, err := s.repo.ListItemsByOrderID(ctx, o.ID)
	if err != nil {
		return err
	}
	if len(items) > 0 && !o.StockReversed {
		releases := make([]InventoryItem, len(items))
		for i, item := range items {
			releases[i] = InventoryItem{ProductID: item.ProductID, Quantity: item.Quantity}
		}
		if err := s.inventory.Restore(ctx, releases, o.StockDeducted); err != nil {
			return fmt.Errorf("restoring inventory on expire: %w", err)
		}
	}

	if s.coupons != nil && o.CouponCode != nil && *o.CouponCode != "" {
		if err := s.coupons.Release(ctx, o.ID); err != nil {
			s.logger.WarnContext(
				ctx,
				"failed to release coupon on expire",
				slog.Any("order_id", o.ID),
				slog.Any("error", err),
			)
		}
	}
	return nil
}
