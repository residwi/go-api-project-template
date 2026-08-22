package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_GetByIDForUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

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

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := r.GetByIDForUser(ctx, userID, orderID)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, items, result.Items)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := r.GetByIDForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		otherUserID := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: domain.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := r.GetByIDForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("list items error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		existingOrder := &domain.Order{ID: orderID, UserID: userID, Status: domain.StatusPaid}
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		dbErr := errors.New("database error")
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := r.GetByIDForUser(ctx, userID, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestReader_ListByUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		cursor := paging.CursorPage{Limit: 10}
		expected := []domain.Order{
			{ID: uuid.New(), UserID: userID, Status: domain.StatusPaid},
			{ID: uuid.New(), UserID: userID, Status: domain.StatusDelivered},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := r.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		cursor := paging.CursorPage{Limit: 10}
		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, nil)

		result, err := r.ListByUser(ctx, userID, cursor)

		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestReader_ListAdmin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}, Status: "paid"}
		expected := []domain.Order{{ID: uuid.New(), Status: domain.StatusPaid}}

		repo.EXPECT().ListAdmin(mock.Anything, params).Return(expected, 1, nil)

		result, total, err := r.ListAdmin(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, 1, total)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}
		dbErr := errors.New("database error")

		repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, dbErr)

		result, total, err := r.ListAdmin(ctx, params)

		assert.Nil(t, result)
		assert.Equal(t, 0, total)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestReader_GetByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success with items", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

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

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(items, nil)

		result, err := r.GetByID(ctx, orderID)

		require.NoError(t, err)
		assert.Equal(t, orderID, result.ID)
		assert.Len(t, result.Items, 2)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := r.GetByID(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("ListItemsByOrderID error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo)

		existingOrder := &domain.Order{ID: orderID, UserID: uuid.New(), Status: domain.StatusPaid}
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		dbErr := errors.New("items query failed")
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, dbErr)

		result, err := r.GetByID(ctx, orderID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestReader_GetSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("reports a shipped order as dispatched and flattens a nil coupon to empty", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		userID := uuid.New()
		repo := NewMockRepository(t)
		r := New(repo)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
			ID:            orderID,
			UserID:        userID,
			Total:         money.New(9000, "IDR"),
			Status:        domain.StatusShipped,
			CouponCode:    nil,
			StockDeducted: true,
		}, nil)

		got, err := r.GetSnapshot(context.Background(), orderID)

		require.NoError(t, err)
		assert.Equal(t, ordercontract.Order{
			ID:            orderID,
			UserID:        userID,
			Total:         money.New(9000, "IDR"),
			Status:        "shipped",
			CouponCode:    "",
			StockDeducted: true,
			Dispatched:    true,
		}, got)
	})
}

func TestReader_GetInfo(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()
	userID := uuid.New()
	repo := NewMockRepository(t)
	r := New(repo)

	repo.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
		ID:     orderID,
		UserID: userID,
		Status: domain.StatusPaid,
	}, nil)

	got, err := r.GetInfo(context.Background(), orderID)

	require.NoError(t, err)
	assert.Equal(t, ordercontract.Order{ID: orderID, UserID: userID, Status: "paid"}, got)
}

func TestReader_ListItemQuantities(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()
	productA := uuid.New()
	productB := uuid.New()

	repo := NewMockRepository(t)
	r := New(repo)
	repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
		{ProductID: productA, Quantity: 2},
		{ProductID: productB, Quantity: 5},
	}, nil)

	got, err := r.ListItemQuantities(context.Background(), orderID)

	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]int{productA: 2, productB: 5}, got)
}

func TestReaderHasDeliveredOrderTakesThreeIDs(t *testing.T) {
	t.Parallel()

	userID, orderID, productID := uuid.New(), uuid.New(), uuid.New()

	repo := NewMockRepository(t)
	r := New(repo)
	repo.EXPECT().HasDeliveredOrder(mock.Anything, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	}).Return(true, nil)

	got, err := r.HasDeliveredOrder(context.Background(), userID, orderID, productID)

	require.NoError(t, err)
	assert.True(t, got)
}
