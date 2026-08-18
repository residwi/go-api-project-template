package place

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("returns existing order when idempotency key matches", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &domain.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         domain.StatusAwaitingPayment,
			Total:          money.New(5000, "USD"),
		}
		items := []domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(existingOrder, nil)
		deps.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		assert.Equal(t, orderID, resp.Order.ID)
		assert.Len(t, resp.Order.Items, 1)
	})

	t.Run("idempotency check error propagates", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)

		idempotencyKey := "idem-key-123"
		dbErr := errors.New("database connection error")
		deps.repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, dbErr)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("empty cart returns ErrCartEmpty", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-empty-cart"

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID:    uuid.New(),
			Items: []cartcontract.CartItem{},
		}, nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrCartEmpty)
	})

	t.Run("unavailable product returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-unavailable"

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "Draft Widget",
					Price:     money.New(1000, "USD"),
					Status:    "draft",
				},
			},
		}, nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("GetCart error propagates", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-cart-error"

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		cartErr := errors.New("cart service error")
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(nil, cartErr)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, cartErr)
	})

	t.Run("success full happy path", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-happy"

		productA := uuid.New()
		productB := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  2,
					Name:      "Widget A",
					Price:     money.New(3000, "USD"),
					Status:    "published",
				},
				{
					ProductID: productB,
					Quantity:  1,
					Name:      "Widget B",
					Price:     money.New(4000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().ReserveBatch(mock.Anything, map[uuid.UUID]int{
			productA: 2,
			productB: 1,
		}).Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(nil)

		deps.payment.EXPECT().
			InitiatePayment(mock.Anything, mock.Anything).
			Return(paymentcontract.ChargeResult{PaymentID: uuid.New()}, nil)
		deps.notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, domain.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, money.New(10000, "USD"), resp.Order.Total)
		assert.Equal(t, money.New(10000, "USD"), resp.Order.Subtotal)
		assert.Equal(t, money.New(0, "USD"), resp.Order.Discount)
		assert.Len(t, resp.Order.Items, 2)
	})

	t.Run("success with coupon applied", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-coupon"

		productA := uuid.New()
		couponCode := "SAVE20"

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(1000), nil)
		deps.repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(1000), int64(4000)).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(nil)

		deps.payment.EXPECT().
			InitiatePayment(mock.Anything, mock.Anything).
			Return(paymentcontract.ChargeResult{PaymentID: uuid.New()}, nil)
		deps.notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := Params{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(5000, "USD"), resp.Order.Subtotal)
		assert.Equal(t, money.New(1000, "USD"), resp.Order.Discount)
		assert.Equal(t, money.New(4000, "USD"), resp.Order.Total)
		assert.Equal(t, &couponCode, resp.Order.CouponCode)
	})

	t.Run("mixed-currency cart is rejected", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-mixed-ccy"

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "USD item",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "EUR item",
					Price:     money.New(5000, "EUR"),
					Status:    "published",
				},
			},
		}, nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("idempotent replay propagates ListItemsByOrderID error", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &domain.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         domain.StatusAwaitingPayment,
			Total:          money.New(5000, "USD"),
		}

		dbErr := errors.New("db error")
		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(existingOrder, nil)
		deps.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("coupon reserve error propagates", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-coupon-err"
		couponCode := "BADCOUPON"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(0), errors.New("invalid coupon"))

		req := Params{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("notification enqueue error is swallowed", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-notif-err"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(nil)

		deps.payment.EXPECT().
			InitiatePayment(mock.Anything, mock.Anything).
			Return(paymentcontract.ChargeResult{PaymentID: uuid.New()}, nil)
		deps.notifications.EXPECT().
			EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).
			Return(errors.New("queue full"))

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, domain.StatusAwaitingPayment, resp.Order.Status)
	})

	t.Run("repo Create error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-create-err"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("inventory Reserve error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-reserve-err"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(errors.New("insufficient stock"))

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("CreateItems error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-items-err"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("cart Clear error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-clear-err"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(errors.New("cache error"))

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("zero total finalizes order without payment", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-zero-total"
		couponCode := "FREE100"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(5000), nil)
		deps.repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(5000), int64(0)).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(nil)

		// Finalized directly instead of left to expire, and no payment is initiated:
		// there is nothing to charge.
		deps.transition.EXPECT().Apply(mock.Anything, mock.Anything, domain.PaidTransition).Return(nil)
		deps.deductor.EXPECT().
			DeductBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := Params{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(0, "USD"), resp.Order.Total)
	})

	t.Run("success with payment initiation failure logs but returns order", func(t *testing.T) {
		t.Parallel()

		cmd, deps := newTestCommand(t)
		idempotencyKey := "idem-pay-fail"

		productA := uuid.New()

		deps.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, apperror.ErrNotFound)
		deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
		deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
			ID: uuid.New(),
			Items: []cartcontract.CartItem{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		deps.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		deps.reserver.EXPECT().
			ReserveBatch(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		deps.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		deps.clearer.EXPECT().Clear(mock.Anything, userID).Return(nil)

		deps.payment.EXPECT().
			InitiatePayment(mock.Anything, mock.Anything).
			Return(paymentcontract.ChargeResult{}, errors.New("gateway down"))
		deps.notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := Params{PaymentMethodID: "pm_test"}
		resp, err := cmd.Execute(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, domain.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, money.New(5000, "USD"), resp.Order.Total)
	})
}

func TestCommand_Execute_RejectsWithdrawnProduct(t *testing.T) {
	t.Parallel()

	cmd, deps := newTestCommand(t)

	userID := uuid.New()
	productID := uuid.New()
	idempotencyKey := "idem-withdrawn-1"

	deps.repo.EXPECT().
		GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
		Return(nil, apperror.ErrNotFound)
	deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
	deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
		ID: uuid.New(),
		Items: []cartcontract.CartItem{
			{
				ProductID: productID, Quantity: 1, Name: "Withdrawn Widget",
				Price: money.New(1000, "USD"), Status: "archived",
			},
		},
	}, nil)

	_, err := cmd.Execute(context.Background(), userID, Params{}, idempotencyKey)

	require.ErrorIs(t, err, apperror.ErrBadRequest)
	assert.Contains(t, err.Error(), "Withdrawn Widget",
		"the error must name the product so the customer can fix their cart")
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	deps.reserver.AssertNotCalled(t, "ReserveBatch", mock.Anything, mock.Anything)
}

func TestCommand_Execute_RejectsUnavailableProduct(t *testing.T) {
	t.Parallel()

	cmd, deps := newTestCommand(t)

	userID := uuid.New()
	idempotencyKey := "idem-unavailable-1"

	deps.repo.EXPECT().
		GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
		Return(nil, apperror.ErrNotFound)
	deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
	deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
		ID: uuid.New(),
		Items: []cartcontract.CartItem{
			{
				ProductID: uuid.New(), Quantity: 1, Name: "", Price: money.New(0, "USD"), Status: "unavailable",
			},
		},
	}, nil)

	_, err := cmd.Execute(context.Background(), userID, Params{}, idempotencyKey)
	require.ErrorIs(t, err, apperror.ErrBadRequest)
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	deps.reserver.AssertNotCalled(t, "ReserveBatch", mock.Anything, mock.Anything)
}

// Both sentinels: money.ErrCurrencyMismatch names the cause but is not a case in
// response.HandleErr, so alone it would be a 500 for what is user input.
//
// Only Lock and Snapshot are expected. The mocks are strict, so that bare set
// forbids ReserveBatch, Create, InitiatePayment and the coupon path -- which is
// what proves the rejection happens in the fold, not merely eventually.
func TestCommand_Execute_RejectsMixedCurrencyCart(t *testing.T) {
	t.Parallel()

	cmd, deps := newTestCommand(t)

	userID := uuid.New()

	deps.repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, "idem-mixed-1").
		Return(nil, apperror.ErrNotFound)
	deps.locker.EXPECT().Lock(mock.Anything, userID).Return(nil)
	deps.carts.EXPECT().Snapshot(mock.Anything, userID).Return(&cartcontract.Cart{
		ID: uuid.New(),
		Items: []cartcontract.CartItem{
			{ProductID: uuid.New(), Quantity: 1, Name: "A", Price: money.New(1000, "USD"), Status: "published"},
			{ProductID: uuid.New(), Quantity: 1, Name: "B", Price: money.New(1000, "IDR"), Status: "published"},
		},
	}, nil)

	_, err := cmd.Execute(context.Background(), userID, Params{}, "idem-mixed-1")
	require.Error(t, err)
	require.ErrorIs(t, err, money.ErrCurrencyMismatch, "the cause must be identifiable")
	require.ErrorIs(t, err, apperror.ErrBadRequest, "a mixed-currency cart is user input -- 400, not 500")
}

type testDeps struct {
	repo          *MockRepository
	locker        *MockCartLocker
	carts         *MockCartReader
	clearer       *MockCartClearer
	reserver      *MockInventoryReserver
	deductor      *MockInventoryDeductor
	payment       *MockPaymentInitiator
	coupons       *MockCouponReserver
	notifications *MockNotificationEnqueuer
	transition    *MockTransitionApplier
}

func newTestCommand(t *testing.T) (*UseCase, testDeps) {
	d := testDeps{
		repo:          NewMockRepository(t),
		locker:        NewMockCartLocker(t),
		carts:         NewMockCartReader(t),
		clearer:       NewMockCartClearer(t),
		reserver:      NewMockInventoryReserver(t),
		deductor:      NewMockInventoryDeductor(t),
		payment:       NewMockPaymentInitiator(t),
		coupons:       NewMockCouponReserver(t),
		notifications: NewMockNotificationEnqueuer(t),
		transition:    NewMockTransitionApplier(t),
	}

	cmd := New(Deps{
		Repo:          d.repo,
		Tx:            testhelper.FakeTxRunner{},
		Locker:        d.locker,
		Carts:         d.carts,
		Clearer:       d.clearer,
		Reserver:      d.reserver,
		Deductor:      d.deductor,
		Payment:       d.payment,
		Coupons:       d.coupons,
		Notifications: d.notifications,
		Transition:    d.transition,
		Logger:        testhelper.DiscardLogger(),
	})
	return cmd, d
}
