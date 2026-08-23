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

		orders := NewMockOrderWriter(t)
		orders.EXPECT().
			Place(t.Context(), userID, orderdomain.NewOrder{Notes: "leave at door"}, "idem-1").
			Return(placed, nil)

		payments := NewMockPaymentCharger(t)
		payments.EXPECT().
			Charge(t.Context(), payment.ChargeRequest{
				OrderID:         orderID,
				Amount:          money.New(2500, "USD"),
				PaymentMethodID: "pm_123",
			}).
			Return(payment.ChargeResult{}, nil)

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testutil.DiscardLogger()})

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

		orders := NewMockOrderWriter(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-2").Return(placed, nil)

		payments := NewMockPaymentCharger(t)
		payments.EXPECT().Charge(t.Context(), mock.Anything).
			Return(payment.ChargeResult{}, errors.New("gateway down"))

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testutil.DiscardLogger()})

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

		orders := NewMockOrderWriter(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-3").Return(placed, nil)

		// No EXPECT on payments: mockery fails the test if Charge is called.
		payments := NewMockPaymentCharger(t)

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testutil.DiscardLogger()})

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

		orders := NewMockOrderWriter(t)
		orders.EXPECT().Place(t.Context(), userID, orderdomain.NewOrder{}, "idem-4").
			Return(nil, apperror.ErrCartEmpty)

		payments := NewMockPaymentCharger(t)

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testutil.DiscardLogger()})

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:          orderdomain.NewOrder{},
			IdempotencyKey: "idem-4",
		})

		require.ErrorIs(t, err, apperror.ErrCartEmpty)
		assert.Nil(t, got)
	})
}

func TestService_RetryPayment(t *testing.T) {
	t.Parallel()

	t.Run("charges an order that is awaiting payment", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderSnapshotReader(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Total:  money.New(4000, "USD"),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)

		payments := NewMockPaymentCharger(t)
		payments.EXPECT().Charge(t.Context(), payment.ChargeRequest{
			OrderID:         orderID,
			Amount:          money.New(4000, "USD"),
			PaymentMethodID: "pm_retry",
		}).Return(payment.ChargeResult{PaymentURL: "https://pay.test/1"}, nil)

		svc := New(Deps{Snapshots: orders, Payments: payments, Logger: testutil.DiscardLogger()})

		got, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.NoError(t, err)
		assert.Equal(t, payment.ChargeResult{PaymentURL: "https://pay.test/1"}, got)
	})

	t.Run("hides another user's order behind not found", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()

		orders := NewMockOrderSnapshotReader(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: uuid.New(),
			Status: string(orderdomain.StatusAwaitingPayment),
		}, nil)

		svc := New(Deps{Snapshots: orders, Payments: NewMockPaymentCharger(t), Logger: testutil.DiscardLogger()})

		_, err := svc.RetryPayment(t.Context(), uuid.New(), orderID, "pm_retry")

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("refuses an order that is not awaiting payment", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderSnapshotReader(t)
		orders.EXPECT().Snapshot(t.Context(), orderID).Return(order.Snapshot{
			ID:     orderID,
			UserID: userID,
			Status: string(orderdomain.StatusPaid),
		}, nil)

		svc := New(Deps{Snapshots: orders, Payments: NewMockPaymentCharger(t), Logger: testutil.DiscardLogger()})

		_, err := svc.RetryPayment(t.Context(), userID, orderID, "pm_retry")

		require.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})
}

func TestService_CancelOrder(t *testing.T) {
	t.Parallel()

	t.Run("cancels the order then its pending payment jobs", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderCanceller(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(nil)

		jobs := NewMockPaymentJobCanceller(t)
		jobs.EXPECT().CancelPendingByOrderID(t.Context(), orderID).Return(nil)

		svc := New(Deps{Cancels: orders, PaymentJobs: jobs, Logger: testutil.DiscardLogger()})

		require.NoError(t, svc.CancelOrder(t.Context(), userID, orderID))
	})

	t.Run("still succeeds when the job cancel fails", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderCanceller(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(nil)

		jobs := NewMockPaymentJobCanceller(t)
		jobs.EXPECT().CancelPendingByOrderID(t.Context(), orderID).Return(errors.New("db down"))

		svc := New(Deps{Cancels: orders, PaymentJobs: jobs, Logger: testutil.DiscardLogger()})

		require.NoError(t, svc.CancelOrder(t.Context(), userID, orderID))
	})

	t.Run("does not touch payment jobs when the cancel is refused", func(t *testing.T) {
		t.Parallel()

		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderCanceller(t)
		orders.EXPECT().CancelByUser(t.Context(), userID, orderID).Return(apperror.ErrOrderCharging)

		svc := New(Deps{
			Cancels:     orders,
			PaymentJobs: NewMockPaymentJobCanceller(t),
			Logger:      testutil.DiscardLogger(),
		})

		require.ErrorIs(t, svc.CancelOrder(t.Context(), userID, orderID), apperror.ErrOrderCharging)
	})
}
