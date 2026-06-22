package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// staleProcessingThreshold is how long an order may sit in payment_processing
// before the sweep treats its charge attempt as dead and reverts it to
// awaiting_payment. It must comfortably exceed a charge job's lease.
const staleProcessingThreshold = 15 * time.Minute

// housekeepingBatchLimit bounds how many orders each sweep pass touches.
const housekeepingBatchLimit = 20

const productStatusPublished = "published"

type CartProvider interface {
	// LockCart takes a row lock on the user's cart for the current transaction so
	// concurrent checkouts of the same cart serialize. Returns core.ErrNotFound
	// when the user has no cart.
	LockCart(ctx context.Context, userID uuid.UUID) error
	GetCart(ctx context.Context, userID uuid.UUID) (*CartSnapshot, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

type CartSnapshot struct {
	ID    uuid.UUID
	Items []CartSnapshotItem
}

type CartSnapshotItem struct {
	ProductID uuid.UUID
	Quantity  int
	Name      string
	Price     int64
	Currency  string
	Status    string
}

// InventoryItem is one product/quantity pair for a batched reserve/restore.
type InventoryItem struct {
	ProductID uuid.UUID
	Quantity  int
}

type InventoryReserver interface {
	ReserveBatch(ctx context.Context, items []InventoryItem) error
	DeductBatch(ctx context.Context, items []InventoryItem) error
	Restore(ctx context.Context, items []InventoryItem, wasDeducted bool) error
}

type PaymentInitiator interface {
	InitiatePayment(ctx context.Context, params InitiatePaymentParams) (PaymentResult, error)
}

type InitiatePaymentParams struct {
	OrderID         uuid.UUID
	Amount          int64
	Currency        string
	PaymentMethodID string
}

type PaymentResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}

type PaymentJobCanceller interface {
	CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error
}

type CouponReserver interface {
	Reserve(ctx context.Context, code string, userID uuid.UUID, orderID uuid.UUID, orderSubtotal int64) (discountAmount int64, err error)
	Release(ctx context.Context, orderID uuid.UUID) error
}

type NotificationEnqueuer interface {
	EnqueueOrderPlaced(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error
}

type Service struct {
	repo          Repository
	pool          *pgxpool.Pool
	cart          CartProvider
	inventory     InventoryReserver
	payment       PaymentInitiator
	paymentCancel PaymentJobCanceller
	coupons       CouponReserver
	notifications NotificationEnqueuer
}

func NewService(
	repo Repository,
	pool *pgxpool.Pool,
	cart CartProvider,
	inventory InventoryReserver,
	payment PaymentInitiator,
	paymentCancel PaymentJobCanceller,
	coupons CouponReserver,
	notifications NotificationEnqueuer,
) *Service {
	return &Service{
		repo:          repo,
		pool:          pool,
		cart:          cart,
		inventory:     inventory,
		payment:       payment,
		paymentCancel: paymentCancel,
		coupons:       coupons,
		notifications: notifications,
	}
}

func (s *Service) PlaceOrder(ctx context.Context, userID uuid.UUID, req PlaceOrderRequest, idempotencyKey string) (*PlaceResponse, error) { //nolint:gocognit,funlen // checkout orchestrates idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction
	existing, err := s.repo.GetByUserIDAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		items, itemErr := s.repo.ListItemsByOrderID(ctx, existing.ID)
		if itemErr != nil {
			return nil, itemErr
		}
		existing.Items = items
		return &PlaceResponse{Order: existing}, nil
	}

	order := &Order{
		UserID:          userID,
		IdempotencyKey:  idempotencyKey,
		Status:          StatusAwaitingPayment,
		CouponCode:      req.CouponCode,
		ShippingAddress: req.ShippingAddress,
		BillingAddress:  req.BillingAddress,
		Notes:           req.Notes,
	}

	var orderItems []Item

	err = database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		// Lock the cart row, then read its contents INSIDE the transaction. This
		// serializes concurrent checkouts of the same cart: a second checkout
		// blocks on the lock until the first commits (clearing the cart), then
		// reads an empty cart and aborts — instead of replaying the same items
		// into a second order. The Idempotency-Key only dedupes retries of one
		// request, not two distinct concurrent checkouts.
		if txErr := s.cart.LockCart(txCtx, userID); txErr != nil {
			if errors.Is(txErr, core.ErrNotFound) {
				return core.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := s.cart.GetCart(txCtx, userID)
		if txErr != nil {
			return txErr
		}
		if len(snapshot.Items) == 0 {
			return core.ErrCartEmpty
		}

		var subtotal int64
		currency := snapshot.Items[0].Currency
		reservations := make([]InventoryItem, len(snapshot.Items))
		orderItems = make([]Item, len(snapshot.Items))
		for i, item := range snapshot.Items {
			if item.Status != productStatusPublished {
				return fmt.Errorf("%w: product %s is not available", core.ErrBadRequest, item.Name)
			}
			// All cart items must share one currency; summing across currencies
			// would produce a meaningless total (and an arbitrary order currency).
			if item.Currency != currency {
				return fmt.Errorf("%w: cart contains mixed currencies", core.ErrBadRequest)
			}
			subtotal += item.Price * int64(item.Quantity)
			reservations[i] = InventoryItem{ProductID: item.ProductID, Quantity: item.Quantity}
		}

		order.SubtotalAmount = subtotal
		order.TotalAmount = subtotal
		order.Currency = currency
		if txErr := s.repo.Create(txCtx, order); txErr != nil {
			return txErr
		}

		if txErr := s.inventory.ReserveBatch(txCtx, reservations); txErr != nil {
			return fmt.Errorf("reserving stock: %w", txErr)
		}

		for i, item := range snapshot.Items {
			orderItems[i] = Item{
				OrderID:     order.ID,
				ProductID:   item.ProductID,
				ProductName: item.Name,
				Price:       item.Price,
				Quantity:    item.Quantity,
				Subtotal:    item.Price * int64(item.Quantity),
			}
		}
		if txErr := s.repo.CreateItems(txCtx, orderItems); txErr != nil {
			return txErr
		}

		if s.coupons != nil && req.CouponCode != nil && *req.CouponCode != "" {
			discount, txErr := s.coupons.Reserve(txCtx, *req.CouponCode, userID, order.ID, subtotal)
			if txErr != nil {
				return txErr
			}
			order.DiscountAmount = discount
			order.TotalAmount = max(subtotal-discount, 0)
			// The order row was inserted with the pre-discount total; persist the
			// discounted amounts so the DB matches what we charge and what payment
			// finalization verifies against.
			if txErr := s.repo.UpdateTotals(txCtx, order.ID, order.DiscountAmount, order.TotalAmount); txErr != nil {
				return txErr
			}
		}

		return s.cart.Clear(txCtx, userID)
	})
	if err != nil {
		return nil, err
	}

	order.Items = orderItems

	if order.TotalAmount > 0 {
		// Discard the result: the order stays awaiting_payment and the gateway
		// webhook (or the charge job) drives it to paid. A failure here is logged,
		// not fatal — the order can be retried or will expire.
		if _, payErr := s.payment.InitiatePayment(ctx, InitiatePaymentParams{
			OrderID:         order.ID,
			Amount:          order.TotalAmount,
			Currency:        order.Currency,
			PaymentMethodID: req.PaymentMethodID,
		}); payErr != nil {
			slog.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				"order_id", order.ID, "error", payErr)
		}
	} else if freeErr := s.finalizeFreeOrder(ctx, order); freeErr != nil {
		// A fully-discounted order has nothing to charge; if it can't be finalized
		// now it stays in awaiting_payment and the expiry sweep cancels it.
		slog.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
			"order_id", order.ID, "error", freeErr)
	}

	if s.notifications != nil {
		if err := s.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			slog.WarnContext(ctx, "failed to enqueue order placed notification", "error", err)
		}
	}

	return &PlaceResponse{Order: order}, nil
}

// finalizeFreeOrder settles a zero-total order (a coupon covered the full
// subtotal) that has no payment: it marks the order paid and deducts the
// reserved stock in one transaction, mirroring FinalizePaymentSuccess for a
// charged order. Apply(PaidTransition) also sets the order's stock_deducted flag
// atomically. Without this the order would sit in awaiting_payment and be
// cancelled by the expiry sweep, so a legitimately free order could never ship.
func (s *Service) finalizeFreeOrder(ctx context.Context, order *Order) error {
	return database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
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

func (s *Service) RetryPayment(ctx context.Context, userID, orderID uuid.UUID, paymentMethodID string) (*PaymentResult, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, core.ErrNotFound
	}
	if order.Status != StatusAwaitingPayment {
		return nil, core.ErrOrderNotPayable
	}

	result, err := s.payment.InitiatePayment(ctx, InitiatePaymentParams{
		OrderID:         order.ID,
		Amount:          order.TotalAmount,
		Currency:        order.Currency,
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
		return core.ErrNotFound
	}

	if order.Status == StatusPaymentProcessing {
		return core.ErrOrderCharging
	}

	if err := s.cancelWithReversal(ctx, order); err != nil {
		return err
	}

	if s.paymentCancel != nil {
		if err := s.paymentCancel.CancelJobsByOrderID(ctx, orderID); err != nil {
			slog.WarnContext(ctx, "failed to cancel payment jobs", "order_id", orderID, "error", err)
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
	return database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if txErr := s.repo.Apply(txCtx, order.ID, CancelledTransition); txErr != nil {
			if errors.Is(txErr, core.ErrConflict) {
				return fmt.Errorf("%w: cannot cancel order in status %s", core.ErrBadRequest, order.Status)
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
				slog.WarnContext(txCtx, "failed to release coupon on cancel", "error", releaseErr)
			}
		}

		return nil
	})
}

// ExpireStale expires awaiting_payment orders whose payment window has lapsed,
// releasing the stock and coupon each reserved — the time-triggered sibling of
// CancelOrder. It is the per-tick housekeeping the payment job runner invokes
// via its Sweeper hook. Each order is handled in its own transaction so a
// per-order failure is logged and the sweep continues.
func (s *Service) ExpireStale(ctx context.Context) error {
	expiryBatchLimit := 20
	orders, err := s.repo.GetExpiredOrders(ctx, expiryBatchLimit)
	if err != nil {
		return fmt.Errorf("getting expired orders: %w", err)
	}
	for _, o := range orders {
		if err := s.expireOne(ctx, o); err != nil {
			slog.ErrorContext(ctx, "failed to expire order", "order_id", o.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) expireOne(ctx context.Context, o Order) error {
	return database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if err := s.repo.Apply(txCtx, o.ID, ExpiredTransition); err != nil {
			if errors.Is(err, core.ErrConflict) {
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
			slog.WarnContext(ctx, "failed to release coupon on expire", "order_id", o.ID, "error", err)
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
			if errors.Is(err, core.ErrConflict) {
				continue // already moved on by another worker
			}
			slog.ErrorContext(ctx, "failed to recover stale processing order", "order_id", o.ID, "error", err)
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
		return nil, core.ErrNotFound
	}

	items, err := s.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = items

	return order, nil
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Order, error) {
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
	case StatusPaid, StatusPaymentProcessing, StatusCancelled, StatusExpired, StatusRefunded:
		return fmt.Errorf("%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update", core.ErrBadRequest, toStatus)
	case StatusAwaitingPayment, StatusProcessing, StatusShipped, StatusDelivered, StatusFulfillmentFailed:
		// Side-effect-free fulfillment markers — allowed to be set directly below
		// (subject to CanTransition); none of these reverse inventory or payment.
	}

	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !CanTransition(order.Status, toStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", core.ErrBadRequest, order.Status, toStatus)
	}

	return s.repo.UpdateStatus(ctx, orderID, order.Status, toStatus)
}

// Apply performs the guarded status transition t (a compare-and-set): it moves
// the order to t.To only if its current status is one of t.From, returning
// core.ErrConflict if nothing matched. It is the single entry point the
// cross-feature wiring adapters call — each names its transition in
// transition.go rather than passing ad-hoc from/to status lists.
func (s *Service) Apply(ctx context.Context, orderID uuid.UUID, t Transition) error {
	return s.repo.Apply(ctx, orderID, t)
}

// ListItemsByOrderID is used by payment service adapter
func (s *Service) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error) {
	return s.repo.ListItemsByOrderID(ctx, orderID)
}

// HasDeliveredOrder reports whether the user has a delivered order containing
// the product. It satisfies review.PurchaseVerifier, letting review confirm a
// purchase through the order module rather than querying the orders schema
// directly from the wiring layer.
func (s *Service) HasDeliveredOrder(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	return s.repo.HasDeliveredOrder(ctx, userID, productID)
}

// SetPaymentDeps sets payment-related dependencies after construction.
// This breaks the circular dependency between order and payment services.
func (s *Service) SetPaymentDeps(payment PaymentInitiator, paymentCancel PaymentJobCanceller) {
	s.payment = payment
	s.paymentCancel = paymentCancel
}
