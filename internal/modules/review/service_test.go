package review

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, orderID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(rv *Review) bool {
			return rv.UserID == userID &&
				rv.ProductID == productID &&
				rv.OrderID == orderID &&
				rv.Rating == 5 &&
				rv.Title == "Great product" &&
				rv.Body == "Really loved it" &&
				rv.Status == "published"
		})).Return(nil)

		req := CreateParams{
			OrderID: orderID,
			Rating:  5,
			Title:   "Great product",
			Body:    "Really loved it",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.NoError(t, err)
		assert.Equal(t, &Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    5,
			Title:     "Great product",
			Body:      "Really loved it",
			Status:    "published",
		}, result)
	})

	t.Run("not delivered", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, nil)

		req := CreateParams{
			OrderID: uuid.New(),
			Rating:  4,
			Title:   "Good",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("purchase verifier error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		verifyErr := errors.New("purchase check failed")
		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, verifyErr)

		req := CreateParams{OrderID: uuid.New(), Rating: 5, Title: "Great"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, verifyErr)
	})

	t.Run("HasUserReviewed error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, dbErr)

		req := CreateParams{OrderID: uuid.New(), Rating: 4, Title: "Good"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("repo create error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		createErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*review.Review")).Return(createErr)

		req := CreateParams{OrderID: uuid.New(), Rating: 3, Title: "OK"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, createErr)
	})

	t.Run("already reviewed", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(true, nil)

		req := CreateParams{
			OrderID: uuid.New(),
			Rating:  3,
			Title:   "OK",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_ListByProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []Review{
			{ID: uuid.New(), ProductID: productID, Rating: 5, Title: "Great"},
			{ID: uuid.New(), ProductID: productID, Rating: 4, Title: "Good"},
		}

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(expected, nil)

		result, err := svc.ListByProduct(ctx, productID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		dbErr := errors.New("query failed")

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(nil, dbErr)

		result, err := svc.ListByProduct(ctx, productID, cursor)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetStats(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		expected := Stats{AverageRating: 4.5, TotalReviews: 10}

		repo.EXPECT().GetStats(mock.Anything, productID).Return(expected, nil)

		result, err := svc.GetStats(ctx, productID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		dbErr := errors.New("stats query failed")

		repo.EXPECT().GetStats(mock.Anything, productID).Return(Stats{}, dbErr)

		result, err := svc.GetStats(ctx, productID)
		assert.Equal(t, Stats{}, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(ctx, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, nil)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(apperror.ErrNotFound)

		err := svc.Delete(ctx, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

// The argument order is userID, orderID, productID -- this pins it, since
// there is no longer a named struct to catch a positional swap at compile time.
func TestService_Create_PassesArgumentsInOrder(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	verifier := NewMockPurchaseVerifier(t)
	svc := NewService(repo, verifier)

	userID := uuid.New()
	productID := uuid.New()
	orderID := uuid.New()

	verifier.EXPECT().HasDeliveredOrder(mock.Anything, userID, orderID, productID).Return(false, nil)

	_, err := svc.Create(context.Background(), userID, productID, CreateParams{
		OrderID: orderID,
		Rating:  5,
		Title:   "Great",
		Body:    "Worked well",
	})
	require.ErrorIs(t, err, apperror.ErrBadRequest, "a non-delivered purchase must be rejected")
}
