package checkout

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestService_PlaceOrder(t *testing.T) {
	t.Parallel()

	t.Run("charges the payment gateway for a payable order", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()
		placed := &orderdomain.Order{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(2500, "USD"),
			Status: orderdomain.StatusAwaitingPayment,
		}

		orders := NewMockOrders(t)
		orders.EXPECT().
			Place(t.Context(), userID, orderdomain.NewOrder{Notes: "leave at door"}, "idem-1").
			Return(placed, true, nil)

		payments := NewMockPayments(t)
		payments.EXPECT().
			Charge(t.Context(), payment.ChargeRequest{
				OrderID:         orderID,
				Amount:          money.New(2500, "USD"),
				PaymentMethodID: "pm_123",
			}).
			Return(payment.ChargeResult{}, nil)

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:           orderdomain.NewOrder{Notes: "leave at door"},
			PaymentMethodID: "pm_123",
			IdempotencyKey:  "idem-1",
		})

		require.NoError(t, err)
		assert.Equal(t, placed, got)
	})

	t.Run("returns the order when the gateway fails, leaving it awaiting payment", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()
		placed := &orderdomain.Order{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(2500, "USD"),
			Status: orderdomain.StatusAwaitingPayment,
		}

		orders := NewMockOrders(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-2").Return(placed, true, nil)

		payments := NewMockPayments(t)
		payments.EXPECT().Charge(t.Context(), mock.Anything).
			Return(payment.ChargeResult{}, errors.New("gateway down"))

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:          orderdomain.NewOrder{},
			IdempotencyKey: "idem-2",
		})

		require.NoError(t, err)
		assert.Equal(t, orderdomain.StatusAwaitingPayment, got.Status)
	})

	t.Run("does not charge a zero-total order", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		placed := &orderdomain.Order{
			ID:     uuid.New(),
			UserID: userID,
			Total:  money.New(0, "USD"),
			Status: orderdomain.StatusPaid,
		}

		orders := NewMockOrders(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-3").Return(placed, true, nil)

		// No EXPECT on payments: mockery fails the test if Charge is called.
		payments := NewMockPayments(t)

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:          orderdomain.NewOrder{},
			IdempotencyKey: "idem-3",
		})

		require.NoError(t, err)
		assert.Equal(t, placed, got)
	})

	t.Run("propagates a placement failure without charging", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-4").
			Return(nil, false, apperror.ErrCartEmpty)

		payments := NewMockPayments(t)

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:          orderdomain.NewOrder{},
			IdempotencyKey: "idem-4",
		})

		require.ErrorIs(t, err, apperror.ErrCartEmpty)
		assert.Nil(t, got)
	})

	t.Run("does not charge again when the idempotency key was a replay", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		// A paid order is what a real replay returns, but the status is not what
		// the guard reads: created=false is, so the order below is deliberately
		// still awaiting_payment with a payable total. Status-inference would
		// charge it a second time.
		replayed := &orderdomain.Order{
			ID:     uuid.New(),
			UserID: userID,
			Total:  money.New(2500, "USD"),
			Status: orderdomain.StatusAwaitingPayment,
		}

		orders := NewMockOrders(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-replay").
			Return(replayed, false, nil)

		// No EXPECT on payments: mockery fails the test if Charge is called.
		payments := NewMockPayments(t)

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:           orderdomain.NewOrder{},
			PaymentMethodID: "pm_123",
			IdempotencyKey:  "idem-replay",
		})

		require.NoError(t, err)
		assert.Equal(t, replayed, got)
	})
}

func TestService_RetryPayment(t *testing.T) {
	t.Parallel()

	t.Run("charges an order that is awaiting payment", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(4000, "USD"),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)
		orders.EXPECT().BeginPaymentAttempt(t.Context(), orderID).Return(nil)

		payments := NewMockPayments(t)
		payments.EXPECT().Charge(t.Context(), payment.ChargeRequest{
			OrderID:         orderID,
			Amount:          money.New(4000, "USD"),
			PaymentMethodID: "pm_retry",
		}).Return(payment.ChargeResult{PaymentURL: "https://pay.test/1"}, nil)

		svc := New(orders, payments, testutil.DiscardLogger())

		got, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.NoError(t, err)
		assert.Equal(t, payment.ChargeResult{PaymentURL: "https://pay.test/1"}, got)
	})

	t.Run("hides another user's order behind not found", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: uuid.New(),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)

		svc := New(orders, NewMockPayments(t), testutil.DiscardLogger())

		_, err := svc.RetryPayment(t.Context(), uuid.New(), orderID, "pm_retry")

		require.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("refuses an order that is not awaiting payment", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Status: string(orderdomain.StatusPaid),
		}, nil)
		orders.EXPECT().BeginPaymentAttempt(t.Context(), orderID).Return(errs.ErrConflict)

		svc := New(orders, NewMockPayments(t), testutil.DiscardLogger())

		_, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	// The snapshot still reports awaiting_payment, so a status check would have
	// charged. Only the lost compare-and-set stops the second retry.
	t.Run("does not charge when a concurrent retry already claimed the order", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(4000, "USD"),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)
		orders.EXPECT().BeginPaymentAttempt(t.Context(), orderID).Return(errs.ErrConflict)

		svc := New(orders, NewMockPayments(t), testutil.DiscardLogger())

		_, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	t.Run("releases the claim when the charge fails", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(4000, "USD"),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)
		orders.EXPECT().BeginPaymentAttempt(t.Context(), orderID).Return(nil)
		orders.EXPECT().MarkAwaitingPayment(t.Context(), orderID).Return(nil)

		payments := NewMockPayments(t)
		payments.EXPECT().Charge(t.Context(), payment.ChargeRequest{
			OrderID:         orderID,
			Amount:          money.New(4000, "USD"),
			PaymentMethodID: "pm_retry",
		}).Return(payment.ChargeResult{}, errors.New("gateway down"))

		svc := New(orders, payments, testutil.DiscardLogger())

		_, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.Error(t, err)
	})
}

func TestService_CancelOrder(t *testing.T) {
	t.Parallel()

	t.Run("cancels the order then its pending payment jobs", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(nil)

		payments := NewMockPayments(t)
		payments.EXPECT().CancelPendingByOrderID(t.Context(), orderID).Return(nil)

		svc := New(orders, payments, testutil.DiscardLogger())

		require.NoError(t, svc.CancelOrder(t.Context(), userID, orderID))
	})

	t.Run("still succeeds when the job cancel fails", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(nil)

		payments := NewMockPayments(t)
		payments.EXPECT().CancelPendingByOrderID(t.Context(), orderID).Return(errors.New("db down"))

		svc := New(orders, payments, testutil.DiscardLogger())

		require.NoError(t, svc.CancelOrder(t.Context(), userID, orderID))
	})

	t.Run("does not touch payment jobs when the cancel is refused", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrders(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(apperror.ErrOrderCharging)

		svc := New(orders, NewMockPayments(t), testutil.DiscardLogger())

		require.ErrorIs(t, svc.CancelOrder(t.Context(), userID, orderID), apperror.ErrOrderCharging)
	})
}
