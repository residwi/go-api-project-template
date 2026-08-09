// Package place is the checkout saga: idempotency check, cart lock, cart read
// inside the lock, availability guard, subtotal with currency check, create,
// reserve, promotion reserve, clear -- all in one transaction. That sequence
// is what test/e2e's checkout saga covers, so it moved here verbatim.
package place

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const productStatusPublished = "published"

type Params struct {
	PaymentMethodID string
	CouponCode      *string
	ShippingAddress *domain.Address
	BillingAddress  *domain.Address
	Notes           string
}

type Result struct {
	Order *domain.Order
}

type Command struct {
	repo          Repository
	tx            database.TxRunner
	cart          CartProvider
	inventory     InventoryReserver
	payment       PaymentInitiator
	coupons       CouponReserver
	notifications NotificationEnqueuer
	transition    TransitionApplier
	logger        *slog.Logger
}

func New(
	repo Repository,
	tx database.TxRunner,
	cart CartProvider,
	inventory InventoryReserver,
	payment PaymentInitiator,
	coupons CouponReserver,
	notifications NotificationEnqueuer,
	transition TransitionApplier,
	log *slog.Logger,
) *Command {
	return &Command{
		repo:          repo,
		tx:            tx,
		cart:          cart,
		inventory:     inventory,
		payment:       payment,
		coupons:       coupons,
		notifications: notifications,
		transition:    transition,
		logger:        log,
	}
}

//nolint:gocognit,funlen // checkout orchestrates idempotency, cart lock+validate, reserve, items, coupon, and clear in one transaction
func (c *Command) Execute(
	ctx context.Context,
	userID uuid.UUID,
	p Params,
	idempotencyKey string,
) (*Result, error) {
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
		return &Result{Order: existing}, nil
	}

	order := &domain.Order{
		UserID:          userID,
		IdempotencyKey:  idempotencyKey,
		Status:          domain.StatusAwaitingPayment,
		CouponCode:      p.CouponCode,
		ShippingAddress: p.ShippingAddress,
		BillingAddress:  p.BillingAddress,
		Notes:           p.Notes,
	}

	var orderItems []domain.Item

	err = c.tx.Run(ctx, func(txCtx context.Context) error {
		// Read the cart INSIDE the transaction, after the lock: a second concurrent
		// checkout then blocks, and reads the emptied cart instead of replaying the
		// same items. Idempotency-Key only dedupes retries of one request.
		if txErr := c.cart.LockCart(txCtx, userID); txErr != nil {
			if errors.Is(txErr, apperror.ErrNotFound) {
				return apperror.ErrCartEmpty
			}
			return txErr
		}

		snapshot, txErr := c.cart.GetSnapshot(txCtx, userID)
		if txErr != nil {
			return txErr
		}
		if len(snapshot.Items) == 0 {
			return apperror.ErrCartEmpty
		}

		// A cart is keyed by product, so this cannot receive a duplicate ProductID.
		reservations := make(map[uuid.UUID]int, len(snapshot.Items))
		orderItems = make([]domain.Item, len(snapshot.Items))
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
		if txErr := c.repo.Create(txCtx, order); txErr != nil {
			return txErr
		}

		if txErr := c.inventory.ReserveBatch(txCtx, reservations); txErr != nil {
			return fmt.Errorf("reserving stock: %w", txErr)
		}

		// A second pass because items need order.ID, which only exists after Create.
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

		if c.coupons != nil && p.CouponCode != nil && *p.CouponCode != "" {
			discount, txErr := c.coupons.Reserve(txCtx, *p.CouponCode, userID, order.ID, subtotal.Amount)
			if txErr != nil {
				return txErr
			}
			// max(..., 0) is this caller's policy, not money's.
			order.Discount = money.New(discount, subtotal.Currency)
			order.Total = money.New(max(subtotal.Amount-discount, 0), subtotal.Currency)
			// The row was inserted pre-discount; payment finalization verifies against
			// what is stored here.
			if txErr := c.repo.UpdateTotals(txCtx, order.ID, order.Discount.Amount, order.Total.Amount); txErr != nil {
				return txErr
			}
		}

		return c.cart.Clear(txCtx, userID)
	})
	if err != nil {
		return nil, err
	}

	order.Items = orderItems

	if order.Total.Amount > 0 {
		// Not fatal: the order stays awaiting_payment for the webhook, a retry, or
		// the expiry sweep.
		if _, payErr := c.payment.InitiatePayment(ctx, paymentcontract.ChargeRequest{
			OrderID:         order.ID,
			Amount:          order.Total,
			PaymentMethodID: p.PaymentMethodID,
		}); payErr != nil {
			c.logger.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				slog.String("order_id", order.ID.String()), slog.String("error", payErr.Error()))
		}
	} else if freeErr := c.finalizeFreeOrder(ctx, order); freeErr != nil {
		c.logger.ErrorContext(ctx, "failed to finalize zero-total order, it stays in awaiting_payment",
			slog.String("order_id", order.ID.String()), slog.String("error", freeErr.Error()))
	}

	if c.notifications != nil {
		if err := c.notifications.EnqueueOrderPlaced(ctx, userID, order.ID); err != nil {
			c.logger.WarnContext(ctx, "failed to enqueue order placed notification", slog.String("error", err.Error()))
		}
	}

	return &Result{Order: order}, nil
}

// finalizeFreeOrder settles a coupon-covered order that has no payment at all.
// Without it the order would sit in awaiting_payment until the expiry sweep
// cancelled it, so a legitimately free order could never ship.
func (c *Command) finalizeFreeOrder(ctx context.Context, order *domain.Order) error {
	return c.tx.Run(ctx, func(txCtx context.Context) error {
		if err := c.transition.Apply(txCtx, order.ID, domain.PaidTransition); err != nil {
			return err
		}
		// One order line per product by construction (see Execute), so no
		// ProductID can collide here.
		deductions := make(map[uuid.UUID]int, len(order.Items))
		for _, item := range order.Items {
			deductions[item.ProductID] = item.Quantity
		}
		return c.inventory.DeductBatch(txCtx, deductions)
	})
}
