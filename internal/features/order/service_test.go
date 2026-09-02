package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/money"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestService_Place(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("returns existing order when idempotency key matches", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

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

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(existingOrder, nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		resp, created, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		require.NoError(t, err)
		assert.Equal(t, orderID, resp.ID)
		assert.Len(t, resp.Items, 1)
		assert.False(t, created, "checkout charges only a created order, so a replay must report false")
	})

	t.Run("idempotency check error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		idempotencyKey := "idem-key-123"
		dbErr := errors.New("database connection error")
		d.repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, dbErr)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("empty cart returns ErrCartEmpty", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-empty-cart"

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID:    uuid.New(),
			Items: []cart.Item{},
		}, nil)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrCartEmpty)
	})

	t.Run("unavailable product returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-unavailable"

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "Draft Widget",
					Price:     money.New(1000, "USD"),
					Status:    "draft",
				},
			},
		}, nil)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("GetCart error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-cart-error"

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		cartErr := errors.New("cart service error")
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(nil, cartErr)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, cartErr)
	})

	t.Run("success full happy path", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-happy"

		productA := uuid.New()
		productB := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
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

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().Reserve(mock.Anything, map[uuid.UUID]int{
			productA: 2,
			productB: 1,
		}).Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.cart.EXPECT().Clear(mock.Anything, userID).Return(nil)
		d.notifications.EXPECT().
			Create(mock.Anything, mock.MatchedBy(func(in notification.NewNotification) bool {
				return in.UserID == userID
			})).Return(nil)

		resp, created, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, created)
		assert.Equal(t, domain.StatusAwaitingPayment, resp.Status)
		assert.Equal(t, money.New(10000, "USD"), resp.Total)
		assert.Equal(t, money.New(10000, "USD"), resp.Subtotal)
		assert.Equal(t, money.New(0, "USD"), resp.Discount)
		assert.Len(t, resp.Items, 2)
	})

	t.Run("success with coupon applied", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-coupon"

		productA := uuid.New()
		couponCode := "SAVE20"

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(1000), nil)
		d.repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(1000), int64(4000)).Return(nil)
		d.cart.EXPECT().Clear(mock.Anything, userID).Return(nil)
		d.notifications.EXPECT().
			Create(mock.Anything, mock.MatchedBy(func(in notification.NewNotification) bool {
				return in.UserID == userID
			})).Return(nil)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{CouponCode: &couponCode}, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(5000, "USD"), resp.Subtotal)
		assert.Equal(t, money.New(1000, "USD"), resp.Discount)
		assert.Equal(t, money.New(4000, "USD"), resp.Total)
		assert.Equal(t, &couponCode, resp.CouponCode)
	})

	t.Run("mixed-currency cart is rejected", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-mixed-ccy"

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
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

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("idempotent replay propagates ListItemsByOrderID error", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &domain.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         domain.StatusAwaitingPayment,
			Total:          money.New(5000, "USD"),
		}

		dbErr := errors.New("db error")
		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(existingOrder, nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("coupon reserve error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-coupon-err"
		couponCode := "BADCOUPON"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(0), errors.New("invalid coupon"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{CouponCode: &couponCode}, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("notification enqueue error is swallowed", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-notif-err"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.cart.EXPECT().Clear(mock.Anything, userID).Return(nil)
		d.notifications.EXPECT().
			Create(mock.Anything, mock.MatchedBy(func(in notification.NewNotification) bool {
				return in.UserID == userID
			})).Return(errors.New("queue full"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, domain.StatusAwaitingPayment, resp.Status)
	})

	t.Run("repo Create error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-create-err"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db error"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("inventory Reserve error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-reserve-err"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(errors.New("insufficient stock"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("CreateItems error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-items-err"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(errors.New("db error"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("cart Clear error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-clear-err"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.cart.EXPECT().Clear(mock.Anything, userID).Return(errors.New("cache error"))

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{}, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("zero total finalizes order without payment", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		idempotencyKey := "idem-zero-total"
		couponCode := "FREE100"

		productA := uuid.New()

		d.repo.EXPECT().
			GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
			Return(nil, errs.ErrNotFound)
		d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
		d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
			ID: uuid.New(),
			Items: []cart.Item{
				{
					ProductID: productA,
					Quantity:  1,
					Name:      "Widget A",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
			},
		}, nil)

		d.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		d.inventory.EXPECT().
			Reserve(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		d.coupons.EXPECT().
			Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).
			Return(int64(5000), nil)
		d.repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(5000), int64(0)).Return(nil)
		d.cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		// Finalized directly instead of left to expire, and no payment is
		// initiated: checkout's payment leg only ever sees an order with a
		// nonzero total, since this branch and that one are mutually exclusive.
		// The named transition is asserted on Repository.Apply, which is where
		// the old TransitionApplier port's expectation moved when that port
		// folded into this Service.
		d.repo.EXPECT().Apply(mock.Anything, mock.Anything, domain.ToPaid).Return(nil)
		d.inventory.EXPECT().
			Deduct(mock.Anything, map[uuid.UUID]int{productA: 1}).
			Return(nil)
		d.notifications.EXPECT().
			Create(mock.Anything, mock.MatchedBy(func(in notification.NewNotification) bool {
				return in.UserID == userID
			})).Return(nil)

		resp, _, err := s.Place(ctx, userID, domain.NewOrder{CouponCode: &couponCode}, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(0, "USD"), resp.Total)
	})
}

func TestService_Place_RejectsWithdrawnProduct(t *testing.T) {
	t.Parallel()

	s, d := newTestService(t)

	userID := uuid.New()
	productID := uuid.New()
	idempotencyKey := "idem-withdrawn-1"

	d.repo.EXPECT().
		GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
		Return(nil, errs.ErrNotFound)
	d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
	d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ProductID: productID, Quantity: 1, Name: "Withdrawn Widget",
				Price: money.New(1000, "USD"), Status: "archived",
			},
		},
	}, nil)

	_, _, err := s.Place(context.Background(), userID, domain.NewOrder{}, idempotencyKey)

	require.ErrorIs(t, err, errs.ErrBadRequest)
	assert.Contains(t, err.Error(), "Withdrawn Widget",
		"the error must name the product so the customer can fix their cart")
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	d.inventory.AssertNotCalled(t, "Reserve", mock.Anything, mock.Anything)
}

func TestService_Place_RejectsUnavailableProduct(t *testing.T) {
	t.Parallel()

	s, d := newTestService(t)

	userID := uuid.New()
	idempotencyKey := "idem-unavailable-1"

	d.repo.EXPECT().
		GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
		Return(nil, errs.ErrNotFound)
	d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
	d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ProductID: uuid.New(), Quantity: 1, Name: "", Price: money.New(0, "USD"), Status: "unavailable",
			},
		},
	}, nil)

	_, _, err := s.Place(context.Background(), userID, domain.NewOrder{}, idempotencyKey)
	require.ErrorIs(t, err, errs.ErrBadRequest)
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	d.inventory.AssertNotCalled(t, "Reserve", mock.Anything, mock.Anything)
}

// Both sentinels: money.ErrCurrencyMismatch names the cause but is not a case in
// response.HandleErr, so alone it would be a 500 for what is user input.
//
// Only Lock and Snapshot are expected. The mocks are strict, so that bare set
// forbids Reserve, Create and the coupon path -- which is what proves
// the rejection happens in the fold, not merely eventually.
func TestService_Place_RejectsMixedCurrencyCart(t *testing.T) {
	t.Parallel()

	s, d := newTestService(t)

	userID := uuid.New()

	d.repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, "idem-mixed-1").
		Return(nil, errs.ErrNotFound)
	d.cart.EXPECT().Lock(mock.Anything, userID).Return(nil)
	d.cart.EXPECT().Snapshot(mock.Anything, userID).Return(&cart.Snapshot{
		ID: uuid.New(),
		Items: []cart.Item{
			{ProductID: uuid.New(), Quantity: 1, Name: "A", Price: money.New(1000, "USD"), Status: "published"},
			{ProductID: uuid.New(), Quantity: 1, Name: "B", Price: money.New(1000, "IDR"), Status: "published"},
		},
	}, nil)

	_, _, err := s.Place(context.Background(), userID, domain.NewOrder{}, "idem-mixed-1")
	require.Error(t, err)
	require.ErrorIs(t, err, money.ErrCurrencyMismatch, "the cause must be identifiable")
	require.ErrorIs(t, err, errs.ErrBadRequest, "a mixed-currency cart is user input -- 400, not 500")
}

func TestService_ListByUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		cursor := paging.CursorPage{Limit: 10}
		expected := []domain.Order{
			{ID: uuid.New(), UserID: userID, Status: domain.StatusPaid},
			{ID: uuid.New(), UserID: userID, Status: domain.StatusDelivered},
		}

		d.repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := s.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		cursor := paging.CursorPage{Limit: 10}
		d.repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, nil)

		result, err := s.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestService_GetForUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaid,
			Total:  money.New(10000, "USD"),
		}
		items := []domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    2,
				Subtotal:    money.New(10000, "USD"),
			},
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := s.GetForUser(ctx, userID, orderID)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, items, result.Items)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		result, err := s.GetForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		otherUserID := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: domain.StatusPaid,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := s.GetForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("list items error", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, UserID: userID, Status: domain.StatusPaid}
		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		dbErr := errors.New("database error")
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := s.GetForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_ListAdmin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}, Status: "paid"}
		expected := []domain.Order{{ID: uuid.New(), Status: domain.StatusPaid}}

		d.repo.EXPECT().ListAdmin(mock.Anything, params).Return(expected, 1, nil)

		result, total, err := s.ListAdmin(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, total)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}
		dbErr := errors.New("database error")

		d.repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, dbErr)

		result, total, err := s.ListAdmin(ctx, params)

		assert.Nil(t, result)
		assert.Equal(t, 0, total)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_Get(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: uuid.New(),
			Status: domain.StatusShipped,
			Total:  money.New(15000, "USD"),
		}
		items := []domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductName: "Gadget A",
				Price:       money.New(7500, "USD"),
				Quantity:    1,
				Subtotal:    money.New(7500, "USD"),
			},
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductName: "Gadget B",
				Price:       money.New(7500, "USD"),
				Quantity:    1,
				Subtotal:    money.New(7500, "USD"),
			},
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := s.Get(ctx, orderID)

		require.NoError(t, err)
		assert.Equal(t, orderID, result.ID)
		assert.Len(t, result.Items, 2)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		result, err := s.Get(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("ListItemsByOrderID error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, UserID: uuid.New(), Status: domain.StatusPaid}
		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		dbErr := errors.New("items query failed")
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := s.Get(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_Snapshot(t *testing.T) {
	t.Parallel()

	t.Run("reports a shipped order as dispatched and flattens a nil coupon to empty", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		userID := uuid.New()
		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
			ID:            orderID,
			UserID:        userID,
			Total:         money.New(9000, "IDR"),
			Status:        domain.StatusShipped,
			CouponCode:    nil,
			StockDeducted: true,
		}, nil)

		got, err := s.Snapshot(context.Background(), orderID)

		require.NoError(t, err)
		assert.Equal(t, Snapshot{
			ID:            orderID,
			UserID:        userID,
			Total:         money.New(9000, "IDR"),
			Status:        "shipped",
			CouponCode:    "",
			StockDeducted: true,
			Dispatched:    true,
		}, got)
	})

	// The deleted GetInfo filled only these three fields off the same read, and
	// shipping -- its only caller -- reads exactly Status and UserID. This is
	// that assertion, moved onto the surviving method: a bare row must still
	// project its identity, with every other field at its zero value rather
	// than at something invented.
	t.Run("projects identity and status for a row with no coupon or totals", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		userID := uuid.New()
		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaid,
		}, nil)

		got, err := s.Snapshot(context.Background(), orderID)

		require.NoError(t, err)
		assert.Equal(t, Snapshot{ID: orderID, UserID: userID, Status: "paid"}, got)
	})

	t.Run("read error propagates", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		got, err := s.Snapshot(context.Background(), orderID)

		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.Equal(t, Snapshot{}, got)
	})
}

func TestService_ListItemQuantities(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()
	productA := uuid.New()
	productB := uuid.New()

	s, d := newTestService(t)
	d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
		{ProductID: productA, Quantity: 2},
		{ProductID: productB, Quantity: 5},
	}, nil)

	got, err := s.ListItemQuantities(context.Background(), orderID)

	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]int{productA: 2, productB: 5}, got)
}

func TestService_HasDeliveredOrder(t *testing.T) {
	t.Parallel()

	userID, orderID, productID := uuid.New(), uuid.New(), uuid.New()

	s, d := newTestService(t)
	d.repo.EXPECT().HasDeliveredOrder(mock.Anything, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	}).Return(true, nil)

	got, err := s.HasDeliveredOrder(context.Background(), userID, orderID, productID)

	require.NoError(t, err)
	assert.True(t, got)
}

func TestService_CancelByUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		otherUserID := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("payment processing returns ErrOrderCharging", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaymentProcessing,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrOrderCharging)
	})

	t.Run("invalid transition from delivered", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusDelivered,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(errs.ErrConflict)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("invalid transition from paid", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaid,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(errs.ErrConflict)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("invalid transition from shipped", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusShipped,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(errs.ErrConflict)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("success cancels awaiting_payment order", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		productA := uuid.New()
		productB := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productA,
				ProductName: "Widget A",
				Price:       money.New(3000, "USD"),
				Quantity:    2,
				Subtotal:    money.New(6000, "USD"),
			},
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productB,
				ProductName: "Widget B",
				Price:       money.New(4000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(4000, "USD"),
			},
		}, nil)
		d.inventory.EXPECT().Restore(mock.Anything, map[uuid.UUID]int{
			productA: 2,
			productB: 1,
		}, inventory.StockReserved).Return(nil)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success releases coupon on cancel", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &domain.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     domain.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   uuid.New(),
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		d.inventory.EXPECT().Restore(mock.Anything, mock.Anything, inventory.StockReserved).Return(nil)
		d.coupons.EXPECT().Release(mock.Anything, orderID).Return(nil)

		err := s.CancelByUser(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("inventory release error fails the cancellation", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		productA := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productA,
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		d.inventory.EXPECT().
			Restore(mock.Anything, mock.Anything, inventory.StockReserved).
			Return(errors.New("inventory error"))

		err := s.CancelByUser(ctx, userID, orderID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "inventory error")
	})

	t.Run("coupon release error is logged but swallowed", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &domain.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     domain.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   uuid.New(),
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		d.inventory.EXPECT().Restore(mock.Anything, mock.Anything, inventory.StockReserved).Return(nil)
		d.coupons.EXPECT().Release(mock.Anything, orderID).Return(errors.New("coupon service down"))

		err := s.CancelByUser(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("Apply error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(errors.New("db error"))

		err := s.CancelByUser(ctx, userID, orderID)

		assert.Error(t, err)
	})

	t.Run("ListItemsByOrderID error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, errors.New("db error"))

		err := s.CancelByUser(ctx, userID, orderID)

		assert.Error(t, err)
	})
}

// TestService_CancelUnpaid runs the same cancelWithReversal path as
// CancelByUser, minus the ownership check: the payment webhook has no caller
// to own the order against.
func TestService_CancelUnpaid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success, no ownership check", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: uuid.New(),
			Status: domain.StatusAwaitingPayment,
		}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{}, nil)
		d.inventory.AssertNotCalled(t, "Restore", mock.Anything, mock.Anything, mock.Anything)

		err := s.CancelUnpaid(ctx, orderID)

		assert.NoError(t, err)
	})

	t.Run("rejects an order already paid", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		existingOrder := &domain.Order{ID: orderID, UserID: uuid.New(), Status: domain.StatusPaid}
		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToCancelled).Return(errs.ErrConflict)

		err := s.CancelUnpaid(ctx, orderID)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})
}

func TestService_ChangeStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success valid transition paid to processing", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusPaid}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToProcessing).Return(nil)

		err := s.ChangeStatus(ctx, orderID, domain.StatusProcessing)

		assert.NoError(t, err)
	})

	t.Run("success valid transition processing to shipped", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusProcessing}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToShipped).Return(nil)

		err := s.ChangeStatus(ctx, orderID, domain.StatusShipped)

		assert.NoError(t, err)
	})

	t.Run("invalid transition awaiting_payment to delivered", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusAwaitingPayment}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := s.ChangeStatus(ctx, orderID, domain.StatusDelivered)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("rejects managed status cancelled without a direct write", func(t *testing.T) {
		t.Parallel()

		// These unwind inventory or money, so ChangeStatus rejects them before
		// any lookup rather than writing the status bare.
		s, _ := newTestService(t)

		err := s.ChangeStatus(ctx, orderID, domain.StatusCancelled)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("rejects managed status refunded without a direct write", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t)

		err := s.ChangeStatus(ctx, orderID, domain.StatusRefunded)

		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		err := s.ChangeStatus(ctx, orderID, domain.StatusProcessing)

		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("update status repo error", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusPaid}

		d.repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToProcessing).Return(errs.ErrConflict)

		err := s.ChangeStatus(ctx, orderID, domain.StatusProcessing)

		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_ExpireStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("expires each stale order, releasing its reservation and coupon", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		coupon := "SAVE10"
		expired := domain.Order{ID: uuid.New(), CouponCode: &coupon}
		productID := uuid.New()

		d.repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]domain.Order{expired}, nil)
		d.repo.EXPECT().Apply(mock.Anything, expired.ID, domain.ToExpired).Return(nil)
		d.repo.EXPECT().ListItemsByOrderID(mock.Anything, expired.ID).
			Return([]domain.Item{{ProductID: productID, Quantity: 2}}, nil)
		d.inventory.EXPECT().
			Restore(mock.Anything, map[uuid.UUID]int{productID: 2}, inventory.StockReserved).
			Return(nil)
		d.coupons.EXPECT().Release(mock.Anything, expired.ID).Return(nil)

		err := s.ExpireStale(ctx)
		require.NoError(t, err)
	})

	t.Run("skips an order another worker already expired", func(t *testing.T) {
		t.Parallel()

		s, d := newTestServiceWithoutCoupons(t)

		expired := domain.Order{ID: uuid.New()}
		d.repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]domain.Order{expired}, nil)
		d.repo.EXPECT().Apply(mock.Anything, expired.ID, domain.ToExpired).Return(errs.ErrConflict)

		err := s.ExpireStale(ctx)
		require.NoError(t, err)
	})

	t.Run("getting expired orders error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		dbErr := errors.New("database error")
		d.repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return(nil, dbErr)

		err := s.ExpireStale(ctx)
		require.ErrorIs(t, err, dbErr)
	})
}

func TestService_RecoverStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("recovers each stale order back to awaiting_payment", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		stale := domain.Order{ID: uuid.New()}
		d.repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, StaleProcessingThreshold, mock.Anything).
			Return([]domain.Order{stale}, nil)
		d.repo.EXPECT().Apply(mock.Anything, stale.ID, domain.ToAwaitingPayment).Return(nil)

		require.NoError(t, s.RecoverStale(ctx))
	})

	t.Run("skips an order another worker already moved on", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		stale := domain.Order{ID: uuid.New()}
		d.repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, StaleProcessingThreshold, mock.Anything).
			Return([]domain.Order{stale}, nil)
		d.repo.EXPECT().
			Apply(mock.Anything, stale.ID, domain.ToAwaitingPayment).
			Return(errs.ErrConflict)

		require.NoError(t, s.RecoverStale(ctx))
	})

	t.Run("getting stale orders error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)

		dbErr := errors.New("database error")
		d.repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, StaleProcessingThreshold, mock.Anything).
			Return(nil, dbErr)

		err := s.RecoverStale(ctx)
		require.ErrorIs(t, err, dbErr)
	})
}

func TestService_Apply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("forwards the transition's to/from to the compare-and-set primitive", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToPaid).Return(nil)

		err := s.Apply(ctx, orderID, domain.ToPaid)

		assert.NoError(t, err)
	})

	t.Run("conflict error propagates", func(t *testing.T) {
		t.Parallel()

		s, d := newTestService(t)
		d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToRefunded).Return(errs.ErrConflict)

		err := s.Apply(ctx, orderID, domain.ToRefunded)

		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

// TestService_MarkPaid stands in for all eight Mark* methods: each is a
// one-line forward to Apply with its named Transition, so proving the wiring
// for one proves the pattern -- the allowed-from set itself is
// domain/state_test.go's TestTransitionGraph.
func TestService_MarkPaid(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()

	s, d := newTestService(t)
	d.repo.EXPECT().Apply(mock.Anything, orderID, domain.ToPaid).Return(nil)

	require.NoError(t, s.MarkPaid(context.Background(), orderID))
}

type testDeps struct {
	repo          *MockRepository
	cart          *MockCart
	inventory     *MockInventory
	coupons       *MockCouponReserver
	notifications *MockNotifications
}

func newTestService(t *testing.T) (*Service, testDeps) {
	t.Helper()
	return newService(t, true)
}

// newTestServiceWithoutCoupons passes a nil CouponReserver, which is a supported
// wiring: the cancel and expire paths guard on it before releasing, and this is
// what exercises that guard rather than the has-no-coupon-code branch beside it.
func newTestServiceWithoutCoupons(t *testing.T) (*Service, testDeps) {
	t.Helper()
	return newService(t, false)
}

func newService(t *testing.T, withCoupons bool) (*Service, testDeps) {
	t.Helper()

	d := testDeps{
		repo:          NewMockRepository(t),
		cart:          NewMockCart(t),
		inventory:     NewMockInventory(t),
		coupons:       NewMockCouponReserver(t),
		notifications: NewMockNotifications(t),
	}

	var coupons CouponReserver = d.coupons
	if !withCoupons {
		var absent CouponReserver
		coupons = absent
	}

	svc := New(
		d.repo,
		testutil.FakeTxRunner{},
		testutil.DiscardLogger(),
		d.cart,
		d.inventory,
		coupons,
		d.notifications,
	)

	return svc, d
}
