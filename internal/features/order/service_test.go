package order_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	mocks "github.com/residwi/go-api-project-template/mocks/order"
)

// noopDBTX satisfies database.DBTX so WithTestTx can seed a tx in context.
type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

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

	svc := order.NewService(repo, nil, cart, inventory, payment, paymentCancel, coupons, notifications)
	return svc, repo, cart, inventory, payment, paymentCancel, coupons, notifications
}

// --- TestCanTransition ---

func TestCanTransition(t *testing.T) {
	t.Run("awaiting_payment to payment_processing is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusAwaitingPayment, order.StatusPaymentProcessing))
	})

	t.Run("awaiting_payment to cancelled is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusAwaitingPayment, order.StatusCancelled))
	})

	t.Run("awaiting_payment to delivered is invalid", func(t *testing.T) {
		assert.False(t, order.CanTransition(order.StatusAwaitingPayment, order.StatusDelivered))
	})

	t.Run("paid to processing is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusPaid, order.StatusProcessing))
	})

	t.Run("paid to cancelled is invalid", func(t *testing.T) {
		assert.False(t, order.CanTransition(order.StatusPaid, order.StatusCancelled))
	})

	t.Run("delivered to refunded is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusDelivered, order.StatusRefunded))
	})

	t.Run("cancelled has no valid transitions", func(t *testing.T) {
		assert.False(t, order.CanTransition(order.StatusCancelled, order.StatusPaid))
		assert.False(t, order.CanTransition(order.StatusCancelled, order.StatusRefunded))
	})

	t.Run("payment_processing to paid is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusPaymentProcessing, order.StatusPaid))
	})

	t.Run("payment_processing to cancelled is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusPaymentProcessing, order.StatusCancelled))
	})

	t.Run("shipped to delivered is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusShipped, order.StatusDelivered))
	})

	t.Run("fulfillment_failed to refunded is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusFulfillmentFailed, order.StatusRefunded))
	})

	t.Run("fulfillment_failed to cancelled is valid", func(t *testing.T) {
		assert.True(t, order.CanTransition(order.StatusFulfillmentFailed, order.StatusCancelled))
	})
}

// --- TestService_RetryPayment ---

func TestService_RetryPayment(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()
	paymentMethodID := "pm_test_123"

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, payment, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusAwaitingPayment,
			TotalAmount: 5000,
			Currency:    "USD",
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		expectedResult := order.PaymentResult{
			PaymentID:  uuid.New(),
			PaymentURL: "https://pay.example.com/checkout",
			Charged:    false,
		}
		payment.EXPECT().InitiatePayment(mock.Anything, order.InitiatePaymentParams{
			OrderID:         orderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: paymentMethodID,
		}).Return(expectedResult, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		require.NoError(t, err)
		assert.Equal(t, &expectedResult, result)
	})

	t.Run("not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
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
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("not payable when status is paid", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrOrderNotPayable)
	})

	t.Run("not payable when status is cancelled", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrOrderNotPayable)
	})

	t.Run("payment initiation fails", func(t *testing.T) {
		svc, repo, _, _, payment, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusAwaitingPayment,
			TotalAmount: 5000,
			Currency:    "USD",
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		paymentErr := errors.New("payment gateway error")
		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{}, paymentErr)

		result, err := svc.RetryPayment(ctx, userID, orderID, paymentMethodID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, paymentErr)
	})
}

// --- TestService_GetByID ---

func TestService_GetByID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusPaid,
			TotalAmount: 10000,
			Currency:    "USD",
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Widget", Price: 5000, Quantity: 2, Subtotal: 10000},
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
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusPaid,
			TotalAmount: 10000,
			Currency:    "USD",
			Items: []order.Item{
				{OrderID: orderID, ProductName: "Widget", Price: 5000, Quantity: 2, Subtotal: 10000},
			},
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		result, err := svc.GetByID(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
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
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("list items error", func(t *testing.T) {
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

// --- TestService_ListByUser ---

func TestService_ListByUser(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		cursor := core.CursorPage{Limit: 10}
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
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		cursor := core.CursorPage{Limit: 10}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, nil)

		result, err := svc.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// --- TestService_AdminListAll ---

func TestService_AdminListAll(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
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

// --- TestService_AdminGetByID ---

func TestService_AdminGetByID(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success with items", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:          orderID,
			UserID:      uuid.New(),
			Status:      order.StatusShipped,
			TotalAmount: 15000,
			Currency:    "USD",
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Gadget A", Price: 7500, Quantity: 1, Subtotal: 7500},
			{ID: uuid.New(), OrderID: orderID, ProductName: "Gadget B", Price: 7500, Quantity: 1, Subtotal: 7500},
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := svc.AdminGetByID(ctx, orderID)

		require.NoError(t, err)
		assert.Equal(t, orderID, result.ID)
		assert.Len(t, result.Items, 2)
	})

	t.Run("not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		result, err := svc.AdminGetByID(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("ListItemsByOrderID error propagates", func(t *testing.T) {
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

// --- TestService_AdminUpdateStatus ---

func TestService_AdminUpdateStatus(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success valid transition paid to processing", func(t *testing.T) {
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
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusDelivered)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("invalid transition paid to cancelled", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusCancelled)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusProcessing)

		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("update status repo error", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusPaid, order.StatusProcessing).Return(core.ErrConflict)

		err := svc.AdminUpdateStatus(ctx, orderID, order.StatusProcessing)

		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

// --- TestService_UpdateStatusMulti ---

func TestService_UpdateStatusMulti(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		fromStatuses := []order.Status{order.StatusAwaitingPayment, order.StatusPaymentProcessing}
		toStatus := order.StatusPaid

		repo.EXPECT().UpdateStatusMulti(mock.Anything, orderID, toStatus, fromStatuses).Return(nil)

		err := svc.UpdateStatusMulti(ctx, orderID, fromStatuses, toStatus)

		assert.NoError(t, err)
	})

	t.Run("conflict error", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		fromStatuses := []order.Status{order.StatusAwaitingPayment}
		toStatus := order.StatusPaid

		repo.EXPECT().UpdateStatusMulti(mock.Anything, orderID, toStatus, fromStatuses).Return(core.ErrConflict)

		err := svc.UpdateStatusMulti(ctx, orderID, fromStatuses, toStatus)

		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

// --- TestService_ListItemsByOrderID ---

func TestService_ListItemsByOrderID(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		expected := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Item A", Price: 1000, Quantity: 2, Subtotal: 2000},
			{ID: uuid.New(), OrderID: orderID, ProductName: "Item B", Price: 3000, Quantity: 1, Subtotal: 3000},
		}

		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(expected, nil)

		result, err := svc.ListItemsByOrderID(ctx, orderID)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, expected, result)
	})

	t.Run("empty list", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, nil)

		result, err := svc.ListItemsByOrderID(ctx, orderID)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// --- TestService_PlaceOrder ---

func TestService_PlaceOrder(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()
	t.Run("returns existing order when idempotency key matches", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &order.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         order.StatusAwaitingPayment,
			TotalAmount:    5000,
			Currency:       "USD",
		}
		items := []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductName: "Widget", Price: 5000, Quantity: 1, Subtotal: 5000},
		}

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		assert.Equal(t, orderID, resp.Order.ID)
		assert.Len(t, resp.Order.Items, 1)
	})

	t.Run("idempotency check error propagates", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		dbErr := errors.New("database connection error")
		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, dbErr)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("empty cart returns ErrCartEmpty", func(t *testing.T) {
		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-empty-cart"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID:    uuid.New(),
			Items: []order.CartSnapshotItem{},
		}, nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrCartEmpty)
	})

	t.Run("unavailable product returns ErrBadRequest", func(t *testing.T) {
		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-unavailable"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "Draft Widget",
					Price:     1000,
					Currency:  "USD",
					Status:    "draft",
				},
			},
		}, nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("GetCart error propagates", func(t *testing.T) {
		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-cart-error"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cartErr := errors.New("cart service error")
		cart.EXPECT().GetCart(mock.Anything, userID).Return(nil, cartErr)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, cartErr)
	})

	t.Run("success full happy path", func(t *testing.T) {
		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-happy"

		productA := uuid.New()
		productB := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 2, Name: "Widget A", Price: 3000, Currency: "USD", Status: "published"},
				{ProductID: productB, Quantity: 1, Name: "Widget B", Price: 4000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 2).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productB, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, int64(10000), resp.Order.TotalAmount)
		assert.Equal(t, int64(10000), resp.Order.SubtotalAmount)
		assert.Equal(t, int64(0), resp.Order.DiscountAmount)
		assert.Len(t, resp.Order.Items, 2)
	})

	t.Run("success with coupon applied", func(t *testing.T) {
		svc, repo, cart, inventory, payment, _, coupons, notifications := newTestService(t)
		idempotencyKey := "idem-coupon"

		productA := uuid.New()
		couponCode := "SAVE20"

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(1000), nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(5000), resp.Order.SubtotalAmount)
		assert.Equal(t, int64(1000), resp.Order.DiscountAmount)
		assert.Equal(t, int64(4000), resp.Order.TotalAmount)
		assert.Equal(t, &couponCode, resp.Order.CouponCode)
	})

	t.Run("idempotent return ignores ListItemsByOrderID error", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		idempotencyKey := "idem-key-123"
		existingOrder := &order.Order{
			ID:             orderID,
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
			Status:         order.StatusAwaitingPayment,
			TotalAmount:    5000,
			Currency:       "USD",
		}

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, errors.New("db error"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(ctx, userID, req, idempotencyKey)

		require.NoError(t, err)
		assert.Equal(t, orderID, resp.Order.ID)
		assert.Nil(t, resp.Order.Items)
	})

	t.Run("coupon reserve error propagates", func(t *testing.T) {
		svc, repo, cart, inventory, _, _, coupons, _ := newTestService(t)
		idempotencyKey := "idem-coupon-err"
		couponCode := "BADCOUPON"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(0), errors.New("invalid coupon"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("notification enqueue error is swallowed", func(t *testing.T) {
		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-notif-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{PaymentID: uuid.New()}, nil)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(errors.New("queue full"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
	})

	t.Run("repo Create error propagates from transaction", func(t *testing.T) {
		svc, repo, cart, _, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-create-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("inventory Reserve error propagates from transaction", func(t *testing.T) {
		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-reserve-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(errors.New("insufficient stock"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("CreateItems error propagates from transaction", func(t *testing.T) {
		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-items-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(errors.New("db error"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("cart Clear error propagates from transaction", func(t *testing.T) {
		svc, repo, cart, inventory, _, _, _, _ := newTestService(t)
		idempotencyKey := "idem-clear-err"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(errors.New("cache error"))

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("zero total skips payment initiation", func(t *testing.T) {
		svc, repo, cart, inventory, _, _, coupons, notifications := newTestService(t)
		idempotencyKey := "idem-zero-total"
		couponCode := "FREE100"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		coupons.EXPECT().Reserve(mock.Anything, couponCode, userID, mock.Anything, int64(5000)).Return(int64(5000), nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		// payment.InitiatePayment should NOT be called (total is 0)
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test", CouponCode: &couponCode}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(0), resp.Order.TotalAmount)
	})

	t.Run("success with payment initiation failure logs but returns order", func(t *testing.T) {
		svc, repo, cart, inventory, payment, _, _, notifications := newTestService(t)
		idempotencyKey := "idem-pay-fail"

		productA := uuid.New()

		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, idempotencyKey).Return(nil, core.ErrNotFound)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{ProductID: productA, Quantity: 1, Name: "Widget A", Price: 5000, Currency: "USD", Status: "published"},
			},
		}, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		inventory.EXPECT().Reserve(mock.Anything, productA, 1).Return(nil)
		repo.EXPECT().CreateItems(mock.Anything, mock.Anything).Return(nil)
		cart.EXPECT().Clear(mock.Anything, userID).Return(nil)

		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(order.PaymentResult{}, errors.New("gateway down"))
		notifications.EXPECT().EnqueueOrderPlaced(mock.Anything, userID, mock.Anything).Return(nil)

		req := order.PlaceOrderRequest{PaymentMethodID: "pm_test"}
		resp, err := svc.PlaceOrder(txCtx, userID, req, idempotencyKey)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, order.StatusAwaitingPayment, resp.Order.Status)
		assert.Equal(t, int64(5000), resp.Order.TotalAmount)
	})
}

// --- TestService_CancelOrder ---

func TestService_CancelOrder(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		otherUserID := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("payment processing returns ErrOrderCharging", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaymentProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrOrderCharging)
	})

	t.Run("invalid transition from delivered", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusDelivered,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("invalid transition from paid", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("invalid transition from shipped", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusShipped,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := svc.CancelOrder(ctx, userID, orderID)

		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("success cancels awaiting_payment order", func(t *testing.T) {
		svc, repo, _, inventory, _, paymentCancel, _, _ := newTestService(t)

		productA := uuid.New()
		productB := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: productA, ProductName: "Widget A", Price: 3000, Quantity: 2, Subtotal: 6000},
			{ID: uuid.New(), OrderID: orderID, ProductID: productB, ProductName: "Widget B", Price: 4000, Quantity: 1, Subtotal: 4000},
		}, nil)
		inventory.EXPECT().Release(mock.Anything, productA, 2).Return(nil)
		inventory.EXPECT().Release(mock.Anything, productB, 1).Return(nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success releases coupon on cancel", func(t *testing.T) {
		svc, repo, _, inventory, _, paymentCancel, coupons, _ := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &order.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     order.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: uuid.New(), ProductName: "Widget", Price: 5000, Quantity: 1, Subtotal: 5000},
		}, nil)
		inventory.EXPECT().Release(mock.Anything, mock.Anything, 1).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success cancels payment jobs best effort", func(t *testing.T) {
		svc, repo, _, _, _, paymentCancel, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(errors.New("redis down"))

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("inventory release error is logged but swallowed", func(t *testing.T) {
		svc, repo, _, inventory, _, paymentCancel, _, _ := newTestService(t)

		productA := uuid.New()
		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: productA, ProductName: "Widget", Price: 5000, Quantity: 1, Subtotal: 5000},
		}, nil)
		inventory.EXPECT().Release(mock.Anything, productA, 1).Return(errors.New("inventory error"))
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("coupon release error is logged but swallowed", func(t *testing.T) {
		svc, repo, _, inventory, _, paymentCancel, coupons, _ := newTestService(t)

		couponCode := "SAVE20"
		existingOrder := &order.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     order.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: uuid.New(), ProductName: "Widget", Price: 5000, Quantity: 1, Subtotal: 5000},
		}, nil)
		inventory.EXPECT().Release(mock.Anything, mock.Anything, 1).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(errors.New("coupon service down"))
		paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("nil paymentCancel skips job cancellation", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		cart := mocks.NewMockCartProvider(t)
		inventory := mocks.NewMockInventoryReserver(t)
		coupons := mocks.NewMockCouponReserver(t)
		notifications := mocks.NewMockNotificationEnqueuer(t)

		svc := order.NewService(repo, nil, cart, inventory, nil, nil, coupons, notifications)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("UpdateStatus error propagates from transaction", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(errors.New("db error"))

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.Error(t, err)
	})

	t.Run("ListItemsByOrderID error propagates from transaction", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _ := newTestService(t)

		existingOrder := &order.Order{
			ID:     orderID,
			UserID: userID,
			Status: order.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		txCtx := database.WithTestTx(ctx, noopDBTX{})

		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusAwaitingPayment, order.StatusCancelled).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, errors.New("db error"))

		err := svc.CancelOrder(txCtx, userID, orderID)

		assert.Error(t, err)
	})
}

// --- TestService_SetPaymentDeps ---

func TestService_SetPaymentDeps(t *testing.T) {
	t.Run("sets payment dependencies and allows retry", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		cart := mocks.NewMockCartProvider(t)
		inventory := mocks.NewMockInventoryReserver(t)
		coupons := mocks.NewMockCouponReserver(t)
		notifications := mocks.NewMockNotificationEnqueuer(t)

		svc := order.NewService(repo, nil, cart, inventory, nil, nil, coupons, notifications)

		payment := mocks.NewMockPaymentInitiator(t)
		paymentCancel := mocks.NewMockPaymentJobCanceller(t)
		svc.SetPaymentDeps(payment, paymentCancel)

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		existingOrder := &order.Order{
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusAwaitingPayment,
			TotalAmount: 5000,
			Currency:    "USD",
		}
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		expectedResult := order.PaymentResult{PaymentID: uuid.New(), Charged: false}
		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).Return(expectedResult, nil)

		result, err := svc.RetryPayment(ctx, userID, orderID, "pm_test")

		require.NoError(t, err)
		assert.Equal(t, &expectedResult, result)
	})
}
