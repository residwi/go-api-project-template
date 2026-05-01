package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const productStatusPublished = "published"

type CartProvider interface {
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

type InventoryReserver interface {
	Reserve(ctx context.Context, productID uuid.UUID, qty int) error
	Release(ctx context.Context, productID uuid.UUID, qty int) error
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

func (s *Service) PlaceOrder(ctx context.Context, userID uuid.UUID, req PlaceOrderRequest, idempotencyKey string) (*PlaceResponse, error) { //nolint:gocognit,funlen
	existing, err := s.repo.GetByUserIDAndIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		items, _ := s.repo.ListItemsByOrderID(ctx, existing.ID)
		existing.Items = items
		return &PlaceResponse{Order: existing}, nil
	}

	snapshot, err := s.cart.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(snapshot.Items) == 0 {
		return nil, core.ErrCartEmpty
	}

	for _, item := range snapshot.Items {
		if item.Status != productStatusPublished {
			return nil, fmt.Errorf("%w: product %s is not available", core.ErrBadRequest, item.Name)
		}
	}

	var subtotal int64
	currency := "USD"
	for _, item := range snapshot.Items {
		subtotal += item.Price * int64(item.Quantity)
		currency = item.Currency
	}
	totalAmount := subtotal

	order := &Order{
		UserID:          userID,
		IdempotencyKey:  idempotencyKey,
		Status:          StatusAwaitingPayment,
		SubtotalAmount:  subtotal,
		DiscountAmount:  0,
		TotalAmount:     totalAmount,
		CouponCode:      req.CouponCode,
		Currency:        currency,
		ShippingAddress: req.ShippingAddress,
		BillingAddress:  req.BillingAddress,
		Notes:           req.Notes,
	}

	var orderItems []Item

	err = database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if txErr := s.repo.Create(txCtx, order); txErr != nil {
			return txErr
		}

		for _, item := range snapshot.Items {
			if txErr := s.inventory.Reserve(txCtx, item.ProductID, item.Quantity); txErr != nil {
				return fmt.Errorf("reserving stock for %s: %w", item.Name, txErr)
			}
		}

		for _, item := range snapshot.Items {
			orderItems = append(orderItems, Item{
				OrderID:     order.ID,
				ProductID:   item.ProductID,
				ProductName: item.Name,
				Price:       item.Price,
				Quantity:    item.Quantity,
				Subtotal:    item.Price * int64(item.Quantity),
			})
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
		}

		if txErr := s.cart.Clear(txCtx, userID); txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	order.Items = orderItems

	if order.TotalAmount > 0 {
		result, err := s.payment.InitiatePayment(ctx, InitiatePaymentParams{
			OrderID:         order.ID,
			Amount:          order.TotalAmount,
			Currency:        order.Currency,
			PaymentMethodID: req.PaymentMethodID,
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				"order_id", order.ID, "error", err)
		} else {
			_ = result // payment initiated, webhook will update status
		}
	}

	if s.notifications != nil {
		if err := s.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			slog.WarnContext(ctx, "failed to enqueue order placed notification", "error", err)
		}
	}

	return &PlaceResponse{Order: order}, nil
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

func (s *Service) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error { //nolint:gocognit
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

	if !CanTransition(order.Status, StatusCancelled) {
		return fmt.Errorf("%w: cannot cancel order in status %s", core.ErrBadRequest, order.Status)
	}

	err = database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if txErr := s.repo.UpdateStatus(txCtx, orderID, order.Status, StatusCancelled); txErr != nil {
			return txErr
		}

		items, txErr := s.repo.ListItemsByOrderID(txCtx, orderID)
		if txErr != nil {
			return txErr
		}
		for _, item := range items {
			if releaseErr := s.inventory.Release(txCtx, item.ProductID, item.Quantity); releaseErr != nil {
				slog.ErrorContext(txCtx, "failed to release inventory on cancel",
					"product_id", item.ProductID, "error", releaseErr)
			}
		}

		if s.coupons != nil && order.CouponCode != nil && *order.CouponCode != "" {
			if releaseErr := s.coupons.Release(txCtx, orderID); releaseErr != nil {
				slog.WarnContext(txCtx, "failed to release coupon on cancel", "error", releaseErr)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if s.paymentCancel != nil {
		if err := s.paymentCancel.CancelJobsByOrderID(ctx, orderID); err != nil {
			slog.WarnContext(ctx, "failed to cancel payment jobs", "order_id", orderID, "error", err)
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
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !CanTransition(order.Status, toStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", core.ErrBadRequest, order.Status, toStatus)
	}

	return s.repo.UpdateStatus(ctx, orderID, order.Status, toStatus)
}

// UpdateStatusMulti is used by payment service adapter
func (s *Service) UpdateStatusMulti(ctx context.Context, orderID uuid.UUID, fromStatuses []Status, toStatus Status) error {
	return s.repo.UpdateStatusMulti(ctx, orderID, toStatus, fromStatuses)
}

// ListItemsByOrderID is used by payment service adapter
func (s *Service) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error) {
	return s.repo.ListItemsByOrderID(ctx, orderID)
}

// SetPaymentDeps sets payment-related dependencies after construction.
// This breaks the circular dependency between order and payment services.
func (s *Service) SetPaymentDeps(payment PaymentInitiator, paymentCancel PaymentJobCanceller) {
	s.payment = payment
	s.paymentCancel = paymentCancel
}
