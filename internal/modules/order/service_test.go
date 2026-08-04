package order_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	cartMocks "github.com/residwi/go-api-project-template/mocks/cart"
	inventoryMocks "github.com/residwi/go-api-project-template/mocks/inventory"
	mocks "github.com/residwi/go-api-project-template/mocks/order"
	productMocks "github.com/residwi/go-api-project-template/mocks/product"
)

func TestService_ExpireStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("expires each stale order, releasing its reservation and coupon", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, inventory, _, _, coupons, _ := newTestService(t)

		coupon := "SAVE10"
		expired := order.Order{ID: uuid.New(), CouponCode: &coupon}
		productID := uuid.New()

		repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]order.Order{expired}, nil)
		repo.EXPECT().Apply(mock.Anything, expired.ID, order.ExpiredTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, expired.ID).
			Return([]order.Item{{ProductID: productID, Quantity: 2}}, nil)
		inventory.EXPECT().
			Restore(mock.Anything, []order.InventoryItem{{ProductID: productID, Quantity: 2}}, false).
			Return(nil)
		coupons.EXPECT().Release(mock.Anything, expired.ID).Return(nil)

		err := svc.ExpireStale(ctx)
		require.NoError(t, err)
	})

	t.Run("skips an order another worker already expired", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		expired := order.Order{ID: uuid.New()}
		repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]order.Order{expired}, nil)
		repo.EXPECT().Apply(mock.Anything, expired.ID, order.ExpiredTransition).Return(apperror.ErrConflict)

		err := svc.ExpireStale(ctx)
		require.NoError(t, err)
	})
}

func TestService_RetryPayment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()
	paymentMethodID := "pm_test_123"

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, payment, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
			Total:  money.New(5000, "USD"),
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		expectedResult := order.PaymentResult{
			PaymentID:  uuid.New(),
			PaymentURL: "https://pay.example.com/checkout",
			Charged:    false,
		}
		payment.EXPECT().InitiatePayment(mock.Anything, order.InitiatePaymentParams{
			OrderID:         orderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: paymentMethodID,
		}).Return(expectedResult, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		require.NoError(t, err)
		assert.Equal(t, &expectedResult, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		otherUserID := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not payable when status is paid", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	t.Run("not payable when status is cancelled", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	t.Run("payment initiation fails", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, payment, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
			Total:  money.New(5000, "USD"),
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		paymentErr := errors.New("payment gateway error")
		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{}, paymentErr)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, paymentErr)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
			Total:  money.New(10000, "USD"),
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 2, Subtotal: money.New(10000, "USD")},
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := svc.GetByID(ctx, userID, orderID)

		require.NoError(t, err)
		require.NotNil(t, result)
		for i := range result.Items {
			result.Items[i].ID = uuid.Nil
		}
		assert.Equal(t, &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
			Total:  money.New(10000, "USD"),
			Items: []order.Item{
				{OrderID: orderID, ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 2, Subtotal: money.New(10000, "USD")},
			},
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := svc.GetByID(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		otherUserID := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.GetByID(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("list items error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		dbErr := errors.New("database error")
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := svc.GetByID(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_ListByUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		cursor := paging.CursorPage{Limit: 10}
		expected := []order.Order{
			{ID: uuid.New(), UserID: userID, Status: order.StatusPaid},
			{ID: uuid.New(), UserID: userID, Status: order.StatusDelivered},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := svc.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		cursor := paging.CursorPage{Limit: 10}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, nil)

		result, err := svc.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestService_AdminListAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		params := order.AdminListParams{Page: 1, PageSize: 20, Status: "paid"}
		expected := []order.Order{
			{ID: uuid.New(), Status: order.StatusPaid},
		}

		repo.EXPECT().ListAdmin(mock.Anything, params).Return(expected, 1, nil)

		result, total, err := svc.AdminListAll(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, total)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		params := order.AdminListParams{Page: 1, PageSize: 20}
		dbErr := errors.New("database error")

		repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, dbErr)

		result, total, err := svc.AdminListAll(ctx, params)

		assert.Nil(t, result)
		assert.Equal(t, 0, total)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_AdminGetByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: uuid.New(),
			Status: order.StatusShipped,
			Total:  money.New(15000, "USD"),
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Gadget A", Price: money.New(7500, "USD"), Quantity: 1, Subtotal: money.New(7500, "USD")},
			{ID: uuid.New(), OrderID: orderID, ProductName: "Gadget B", Price: money.New(7500, "USD"), Quantity: 1, Subtotal: money.New(7500, "USD")},
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := svc.AdminGetByID(ctx, orderID)

		require.NoError(t, err)
		assert.Equal(t, orderID, result.ID)
		assert.Len(t, result.Items, 2)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := svc.AdminGetByID(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("ListItemsByOrderID error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: uuid.New(),
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		dbErr := errors.New("items query failed")
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := svc.AdminGetByID(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_AdminUpdateStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success valid transition paid to processing", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusPaid, order.StatusProcessing).Return(nil)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusProcessing)

		assert.NoError(t, err)
	})

	t.Run("success valid transition processing to shipped", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusProcessing, order.StatusShipped).Return(nil)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusShipped)

		assert.NoError(t, err)
	})

	t.Run("invalid transition awaiting_payment to delivered", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusDelivered)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("rejects managed status cancelled without a direct write", func(t *testing.T) {
		t.Parallel()

		// cancelled/expired/refunded/paid unwind inventory or money and must go
		// through their owning flow; AdminUpdateStatus rejects them before any
		// lookup rather than writing the status bare.
		svc, _, _, _, _, _, _, _ := newTestService(t)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusCancelled)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("rejects managed status refunded without a direct write", func(t *testing.T) {
		t.Parallel()

		svc, _, _, _, _, _, _, _ := newTestService(t)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusRefunded)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusProcessing)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update status repo error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusPaid, order.StatusProcessing).Return(apperror.ErrConflict)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusProcessing)

		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_Apply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("forwards the transition's to/from to the compare-and-set primitive", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().Apply(mock.Anything, orderID, order.PaidTransition).Return(nil)

		err := svc.Apply(ctx, orderID, order.PaidTransition)

		assert.NoError(t, err)
	})

	t.Run("conflict error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().Apply(mock.Anything, orderID, order.RefundTransition).Return(apperror.ErrConflict)

		err := svc.Apply(ctx, orderID, order.RefundTransition)

		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_ListItemsByOrderID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		expected := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Item A", Price: money.New(1000, "USD"), Quantity: 2, Subtotal: money.New(2000, "USD")},
			{ID: uuid.New(), OrderID: orderID, ProductName: "Item B", Price: money.New(3000, "USD"), Quantity: 1, Subtotal: money.New(3000, "USD")},
		}

		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(expected, nil)

		result, err := svc.ListItemsByOrderID(ctx, orderID)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, nil)

		result, err := svc.ListItemsByOrderID(ctx, orderID)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestService_PlaceOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()
	t.Run("returns existing order when idempotency key matches", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &order.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         order.StatusAwaitingPayment,
			Total:          money.New(5000, "USD"),
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 1, Subtotal: money.New(5000, "USD")},
		}

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		assert.Equal(t, orderID, resp.Order.ID)
		assert.Len(t, resp.Order.Items, 1)
	})

	t.Run("idempotency check error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		dbErr := errors.New("database connection error")
		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, dbErr)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("empty cart returns ErrCartEmpty", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-empty-cart"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID:    uuid.New(),
			Items: []order.CartSnapshotItem{},
		}, nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrCartEmpty)
	})

	t.Run("unavailable product returns ErrBadRequest", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-unavailable"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "Draft Widget",
					Price:     money.New(1000, "USD"),
					Status:    "draft",
				},
			},
		}, nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("GetCart error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-cart-error"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cartErr := errors.New("cart service error")
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(nil, cartErr)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, cartErr)
	})

	t.Run("success full happy path", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-happy"

		productA := uuid.New()
		productB := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 2, Name: "Widget A", Price: money.New(3000, "USD"), Status: "published"},
				{ProductID: productB, Quantity: 1, Name: "Widget B", Price: money.New(4000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{
			{ProductID: productA, Quantity: 2},
			{ProductID: productB, Quantity: 1},
		}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, money.New(10000, "USD"), resp.Order.Total)
		assert.Equal(t, money.New(10000, "USD"), resp.Order.Subtotal)
		assert.Equal(t, money.New(0, "USD"), resp.Order.Discount)
		assert.Len(t, resp.Order.Items, 2)
	})

	t.Run("success with coupon applied", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, payment, _, coupons, notifications := newTestService(t)
		idempotencyKey := "idem-coupon"

		productA := uuid.New()
		couponCode := "SAVE20"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(1000), nil)
		repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(1000), int64(4000)).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(5000, "USD"), resp.Order.Subtotal)
		assert.Equal(t, money.New(1000, "USD"), resp.Order.Discount)
		assert.Equal(t, money.New(4000, "USD"), resp.Order.Total)
		assert.Equal(t, &couponCode, resp.Order.CouponCode)
	})

	t.Run("mixed-currency cart is rejected", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-mixed-ccy"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: uuid.New(), Quantity: 1, Name: "USD item", Price: money.New(5000, "USD"), Status: "published"},
				{ProductID: uuid.New(), Quantity: 1, Name: "EUR item", Price: money.New(5000, "EUR"), Status: "published"},
			},
		}, nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("idempotent replay propagates ListItemsByOrderID error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &order.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         order.StatusAwaitingPayment,
			Total:          money.New(5000, "USD"),
		}

		dbErr := errors.New("db error")
		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("coupon reserve error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, _, _, coupons, _ := newTestService(t)
		idempotencyKey := "idem-coupon-err"
		couponCode := "BADCOUPON"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(0), errors.New("invalid coupon"))

		req := order.PlaceParams{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("notification enqueue error is swallowed", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-notif-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(errors.New("queue full"))

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
	})

	t.Run("repo Create error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-create-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("inventory Reserve error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-reserve-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(errors.New("insufficient stock"))

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("CreateItems error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-items-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("cart Clear error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-clear-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(errors.New("cache error"))

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("zero total finalizes order without payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, payment, _, coupons, notifications := newTestService(t)
		idempotencyKey := "idem-zero-total"
		couponCode := "FREE100"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(5000), nil)
		repo.EXPECT().UpdateTotals(mock.Anything, mock.Anything, int64(5000), int64(0)).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		// A fully-discounted order is finalized directly — marked paid and its
		// reserved stock deducted — instead of being left to expire. Payment is
		// never initiated (there is nothing to charge).
		repo.EXPECT().Apply(mock.Anything, mock.Anything, order.PaidTransition).Return(nil)
		inventory.EXPECT().DeductBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)
		_ = payment

		req := order.PlaceParams{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, money.New(0, "USD"), resp.Order.Total)
	})

	t.Run("success with payment initiation failure logs but returns order", func(t *testing.T) {
		t.Parallel()

		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-pay-fail"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: money.New(5000, "USD"), Status: "published"},
			},
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().ReserveBatch(mock.Anything, []order.InventoryItem{{ProductID: productA, Quantity: 1}}).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{}, errors.New("gateway down"))
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceParams{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, money.New(5000, "USD"), resp.Order.Total)
	})
}

func TestService_CancelOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		otherUserID := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("payment processing returns ErrOrderCharging", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaymentProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrOrderCharging)
	})

	t.Run("invalid transition from delivered", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusDelivered,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(apperror.ErrConflict)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("invalid transition from paid", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(apperror.ErrConflict)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("invalid transition from shipped", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusShipped,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(apperror.ErrConflict)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("success cancels awaiting_payment order", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, inventory, _, paymentCancel, _, _ := newTestService(t)

		productA := uuid.New()
		productB := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: productA, ProductName: "Widget A", Price: money.New(3000, "USD"), Quantity: 2, Subtotal: money.New(6000, "USD")},
			{ID: uuid.New(), OrderID: orderID, ProductID: productB, ProductName: "Widget B", Price: money.New(4000, "USD"), Quantity: 1, Subtotal: money.New(4000, "USD")},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, []order.InventoryItem{
			{ProductID: productA, Quantity: 2},
			{ProductID: productB, Quantity: 1},
		}, false).Return(nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success releases coupon on cancel", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, inventory, _, paymentCancel, coupons, _ := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &order.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     order.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: uuid.New(), ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 1, Subtotal: money.New(5000, "USD")},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, mock.Anything, false).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success cancels payment jobs best effort", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, paymentCancel, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(errors.New("redis down"))

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("inventory release error fails the cancellation", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, inventory, _, _, _, _ := newTestService(t)

		productA := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: productA, ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 1, Subtotal: money.New(5000, "USD")},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, mock.Anything, false).Return(errors.New("inventory error"))
		// The release failure rolls back the cancellation (the tx returns the error),
		// so payment-job cancellation is never reached.

		err := svc.CancelOrder(ctx, userID, orderID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "inventory error")
	})

	t.Run("coupon release error is logged but swallowed", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, inventory, _, paymentCancel, coupons, _ := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &order.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     order.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: uuid.New(), ProductName: "Widget", Price: money.New(5000, "USD"), Quantity: 1, Subtotal: money.New(5000, "USD")},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, mock.Anything, false).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(errors.New("coupon service down"))
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("nil paymentCancel skips job cancellation", func(t *testing.T) {
		t.Parallel()

		repo := mocks.NewMockRepository(t)
		cart := mocks.NewMockCartProvider(t)
		inventory := mocks.NewMockInventoryReserver(t)
		coupons := mocks.NewMockCouponReserver(t)
		notifications := mocks.NewMockNotificationEnqueuer(t)

		svc := order.NewService(repo, testhelper.FakeTxRunner{}, cart, inventory, nil, nil, coupons, notifications)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("Apply error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(errors.New("db error"))

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.Error(t, err)
	})

	t.Run("ListItemsByOrderID error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		repo.EXPECT().Apply(mock.Anything, orderID, order.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, errors.New("db error"))

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.Error(t, err)
	})
}

func TestService_SetPaymentDeps(t *testing.T) {
	t.Parallel()

	t.Run("sets payment dependencies and allows retry", func(t *testing.T) {
		t.Parallel()

		repo := mocks.NewMockRepository(t)
		cart := mocks.NewMockCartProvider(t)
		inventory := mocks.NewMockInventoryReserver(t)
		coupons := mocks.NewMockCouponReserver(t)
		notifications := mocks.NewMockNotificationEnqueuer(t)

		svc := order.NewService(repo, testhelper.FakeTxRunner{}, cart, inventory, nil, nil, coupons, notifications)

		payment := mocks.NewMockPaymentInitiator(t)
		paymentCancel := mocks.NewMockPaymentJobCanceller(t)
		svc.SetPaymentDeps(payment, paymentCancel)

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
			Total:  money.New(5000, "USD"),
		}
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		expectedResult := order.PaymentResult{PaymentID: uuid.New(), Charged: false}
		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(expectedResult, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, "pm_test")

		require.NoError(t, err)
		assert.Equal(t, &expectedResult, result)
	})
}

// TestService_PlaceOrder_RejectsWithdrawnProduct pins the behaviour Task 5
// changed. Cart used to drop a soft-deleted line via its JOIN, so checkout
// silently proceeded without it; cart now surfaces the line carrying its status,
// and PlaceOrder's existing guard refuses the whole order by name.
func TestService_PlaceOrder_RejectsWithdrawnProduct(t *testing.T) {
	t.Parallel()

	svc, repo, cartProvider, inventory, _, _, _, _ := newTestService(t)

	userID := uuid.New()
	productID := uuid.New()
	idempotencyKey := "idem-withdrawn-1"

	repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
	cartProvider.EXPECT().LockCart(mock.Anything, userID).Return(nil)
	cartProvider.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
		ID: uuid.New(),
		Items: []order.CartSnapshotItem{
			{
				ProductID: productID, Quantity: 1, Name: "Withdrawn Widget",
				Price: money.New(1000, "USD"), Status: "archived",
			},
		},
	}, nil)

	_, err := svc.PlaceOrder(context.Background(), userID,
		order.PlaceParams{}, idempotencyKey)

	require.ErrorIs(t, err, apperror.ErrBadRequest)
	assert.Contains(t, err.Error(), "Withdrawn Widget",
		"the error must name the product so the customer can fix their cart")
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	inventory.AssertNotCalled(t, "ReserveBatch", mock.Anything, mock.Anything)
}

// TestService_PlaceOrder_RejectsUnavailableProduct covers the case where the
// product record is gone entirely -- cart supplies Status "unavailable".
func TestService_PlaceOrder_RejectsUnavailableProduct(t *testing.T) {
	t.Parallel()

	svc, repo, cartProvider, inventory, _, _, _, _ := newTestService(t)

	userID := uuid.New()
	idempotencyKey := "idem-unavailable-1"

	repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, apperror.ErrNotFound)
	cartProvider.EXPECT().LockCart(mock.Anything, userID).Return(nil)
	cartProvider.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
		ID: uuid.New(),
		Items: []order.CartSnapshotItem{
			{
				ProductID: uuid.New(), Quantity: 1, Name: "", Price: money.New(0, "USD"), Status: "unavailable",
			},
		},
	}, nil)

	_, err := svc.PlaceOrder(context.Background(), userID,
		order.PlaceParams{}, idempotencyKey)
	require.ErrorIs(t, err, apperror.ErrBadRequest)
	// Direct and intentional, rather than incidental: the guard must reject
	// before any stock is reserved, not merely happen to fail elsewhere first.
	inventory.AssertNotCalled(t, "ReserveBatch", mock.Anything, mock.Anything)
}

// TestService_PlaceOrder_RejectsSoftDeletedProduct exercises the real
// product -> cart chain (product.Service.GetByIDsIncludingDeleted through the
// actual bootstrap.productLookupAdapter) instead of a hand-built
// CartSnapshotItem. Task 7's two tests above only ever supply the
// guard-tripping status directly ("archived", "unavailable"), so neither
// would have caught cart's adapter forwarding a soft-deleted product's stale
// status='published' straight through -- this is the shape that would have.
func TestService_PlaceOrder_RejectsSoftDeletedProduct(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	cartID := uuid.New()
	productID := uuid.New()
	idempotencyKey := "idem-soft-deleted-1"
	deletedAt := time.Now()

	orderRepo := mocks.NewMockRepository(t)
	orderInventory := mocks.NewMockInventoryReserver(t)

	productRepo := productMocks.NewMockRepository(t)
	productRepo.EXPECT().GetByIDsIncludingDeleted(mock.Anything, []uuid.UUID{productID}).
		Return([]product.Product{
			{
				ID: productID, Name: "Withdrawn Widget", Price: money.New(0, "USD"),
				Status: product.StatusPublished, DeletedAt: &deletedAt,
			},
		}, nil)

	invRepo := inventoryMocks.NewMockRepository(t)
	invRepo.EXPECT().GetLevels(mock.Anything, []uuid.UUID{productID}).
		Return(map[uuid.UUID]inventory.Stock{}, nil)
	productSvc := bootstrap.NewProductService(productRepo, inventory.NewService(invRepo))

	cartRepo := cartMocks.NewMockRepository(t)
	cartRepo.EXPECT().GetCartForLock(mock.Anything, userID).Return(cartID, nil)
	cartRepo.EXPECT().GetCart(mock.Anything, userID).Return(&cart.Cart{
		ID:    cartID,
		Items: []cart.Item{{ProductID: productID, Quantity: 1}},
	}, nil)
	// Only reached if the guard fails to reject: exercised in a pre-fix run,
	// never called once the guard correctly rejects.
	cartRepo.EXPECT().Clear(mock.Anything, userID).Return(nil).Maybe()

	cartSvc := bootstrap.NewCartService(cartRepo, testhelper.FakeTxRunner{}, productSvc, 50)

	// Everything below the guard is lenient (.Maybe()): a pre-fix run reaches
	// and exercises these; a correctly-rejecting run never calls them.
	orderRepo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).
		Return(nil, apperror.ErrNotFound)
	orderRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Maybe()
	orderRepo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil).Maybe()
	orderRepo.EXPECT().Apply(mock.Anything, mock.Anything, order.PaidTransition).Return(nil).Maybe()
	orderInventory.EXPECT().ReserveBatch(mock.Anything, mock.Anything).Return(nil).Maybe()
	orderInventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).Return(nil).Maybe()

	svc := order.NewService(orderRepo, testhelper.FakeTxRunner{},
		realCartProvider{svc: cartSvc}, orderInventory, nil, nil, nil, nil)

	_, err := svc.PlaceOrder(context.Background(), userID, order.PlaceParams{}, idempotencyKey)

	require.ErrorIs(t, err, apperror.ErrBadRequest,
		"a soft-deleted product (status still 'published', deleted_at set) must be flagged "+
			"unavailable by cart's adapter and rejected here, not pass through as sellable")
	orderInventory.AssertNotCalled(t, "ReserveBatch", mock.Anything, mock.Anything)
}

// TestService_PlaceOrder_RejectsMixedCurrencyCart pins BOTH sentinels on the
// rejection. money.ErrCurrencyMismatch names the cause, but it is not a case in
// response.HandleErr, so surfacing it alone would fall through to a 500 -- and a
// mixed-currency cart is user input. apperror.ErrBadRequest is what keeps the
// 400 the hand-rolled currency loop used to produce.
//
// Beyond the idempotency lookup PlaceOrder always does first, only LockCart and
// GetCart are expected: the mocks are strict, so the bare set forbids
// ReserveBatch, Create, InitiatePayment and the coupon path. That is what proves
// the rejection happens in the fold, before any of them, rather than merely
// eventually.
func TestService_PlaceOrder_RejectsMixedCurrencyCart(t *testing.T) {
	t.Parallel()

	svc, repo, cartProvider, _, _, _, _, _ := newTestService(t)

	userID := uuid.New()

	repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, "idem-mixed-1").
		Return(nil, apperror.ErrNotFound)
	cartProvider.EXPECT().LockCart(mock.Anything, userID).Return(nil)
	cartProvider.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
		ID: uuid.New(),
		Items: []order.CartSnapshotItem{
			{ProductID: uuid.New(), Quantity: 1, Name: "A", Price: money.New(1000, "USD"), Status: "published"},
			{ProductID: uuid.New(), Quantity: 1, Name: "B", Price: money.New(1000, "IDR"), Status: "published"},
		},
	}, nil)

	_, err := svc.PlaceOrder(context.Background(), userID, order.PlaceParams{}, "idem-mixed-1")
	require.Error(t, err)
	require.ErrorIs(t, err, money.ErrCurrencyMismatch, "the cause must be identifiable")
	require.ErrorIs(t, err, apperror.ErrBadRequest, "a mixed-currency cart is user input -- 400, not 500")
}

// realCartProvider mirrors internal/bootstrap's unexported cartProviderAdapter
// (it is not the fix under test, just the trivial Cart -> CartSnapshot
// mapping), so this test can drive a real cart.Service -- and, critically, the
// real bootstrap.productLookupAdapter it wraps -- without reaching into bootstrap's
// unexported types.
type realCartProvider struct{ svc *cart.Service }

func (a realCartProvider) LockCart(ctx context.Context, userID uuid.UUID) error {
	return a.svc.LockCart(ctx, userID)
}

func (a realCartProvider) GetCart(ctx context.Context, userID uuid.UUID) (*order.CartSnapshot, error) {
	c, err := a.svc.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	snap := &order.CartSnapshot{ID: c.ID}
	for _, item := range c.Items {
		si := order.CartSnapshotItem{ProductID: item.ProductID, Quantity: item.Quantity}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}

func (a realCartProvider) Clear(ctx context.Context, userID uuid.UUID) error {
	return a.svc.Clear(ctx, userID)
}

func newTestService(t *testing.T) (
	*order.Service,
	*mocks.MockRepository,
	*mocks.MockCartProvider,
	*mocks.MockInventoryReserver,
	*mocks.MockPaymentInitiator,
	*mocks.MockPaymentJobCanceller,
	*mocks.MockCouponReserver,
	*mocks.MockNotificationEnqueuer,
) {
	repo := mocks.NewMockRepository(t)
	cart := mocks.NewMockCartProvider(t)
	inventory := mocks.NewMockInventoryReserver(t)
	payment := mocks.NewMockPaymentInitiator(t)
	paymentCancel := mocks.NewMockPaymentJobCanceller(t)
	coupons := mocks.NewMockCouponReserver(t)
	notifications := mocks.NewMockNotificationEnqueuer(t)

	svc := order.NewService(repo, testhelper.FakeTxRunner{}, cart, inventory, payment, paymentCancel, coupons, notifications)
	return svc, repo, cart, inventory, payment, paymentCancel, coupons, notifications
}
