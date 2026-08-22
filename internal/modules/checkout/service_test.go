package checkout

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
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
			Charge(t.Context(), paymentcontract.ChargeRequest{
				OrderID:         orderID,
				Amount:          money.New(2500, "USD"),
				PaymentMethodID: "pm_123",
			}).
			Return(paymentcontract.ChargeResult{}, nil)

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testhelper.DiscardLogger()})

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
			Return(paymentcontract.ChargeResult{}, errors.New("gateway down"))

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testhelper.DiscardLogger()})

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

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testhelper.DiscardLogger()})

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

		svc := New(Deps{Orders: orders, Payments: payments, Logger: testhelper.DiscardLogger()})

		got, err := svc.PlaceOrder(t.Context(), userID, PlaceOrderInput{
			Order:          orderdomain.NewOrder{},
			IdempotencyKey: "idem-4",
		})

		require.ErrorIs(t, err, apperror.ErrCartEmpty)
		assert.Nil(t, got)
	})
}
