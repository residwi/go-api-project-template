package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/review"
	mocks "github.com/residwi/go-api-project-template/mocks/review"
)

func TestService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(rv *review.Review) bool {
			return rv.UserID == userID &&
				rv.ProductID == productID &&
				rv.OrderID == orderID &&
				rv.Rating == 5 &&
				rv.Title == "Great product" &&
				rv.Body == "Really loved it" &&
				rv.Status == "published"
		})).Return(nil)

		req := review.CreateReviewRequest{
			OrderID: orderID,
			Rating:  5,
			Title:   "Great product",
			Body:    "Really loved it",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.NoError(t, err)
		assert.Equal(t, &review.Review{
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
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(false, nil)

		req := review.CreateReviewRequest{
			OrderID: uuid.New(),
			Rating:  4,
			Title:   "Good",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("purchase verifier error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		verifyErr := errors.New("purchase check failed")
		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(false, verifyErr)

		req := review.CreateReviewRequest{OrderID: uuid.New(), Rating: 5, Title: "Great"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, verifyErr)
	})

	t.Run("HasUserReviewed error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(true, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, dbErr)

		req := review.CreateReviewRequest{OrderID: uuid.New(), Rating: 4, Title: "Good"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("repo create error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		createErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*review.Review")).Return(createErr)

		req := review.CreateReviewRequest{OrderID: uuid.New(), Rating: 3, Title: "OK"}
		result, err := svc.Create(ctx, userID, productID, req)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, createErr)
	})

	t.Run("already reviewed", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		purchase := mocks.NewMockPurchaseVerifier(t)
		svc := review.NewService(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(true, nil)

		req := review.CreateReviewRequest{
			OrderID: uuid.New(),
			Rating:  3,
			Title:   "OK",
		}

		result, err := svc.Create(ctx, userID, productID, req)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestService_ListByProduct(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		cursor := core.CursorPage{Limit: 20}
		expected := []review.Review{
			{ID: uuid.New(), ProductID: productID, Rating: 5, Title: "Great"},
			{ID: uuid.New(), ProductID: productID, Rating: 4, Title: "Good"},
		}

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(expected, nil)

		result, err := svc.ListByProduct(ctx, productID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		cursor := core.CursorPage{Limit: 20}
		dbErr := errors.New("query failed")

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(nil, dbErr)

		result, err := svc.ListByProduct(ctx, productID, cursor)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetStats(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		expected := review.Stats{AverageRating: 4.5, TotalReviews: 10}

		repo.EXPECT().GetStats(mock.Anything, productID).Return(expected, nil)

		result, err := svc.GetStats(ctx, productID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		productID := uuid.New()
		dbErr := errors.New("stats query failed")

		repo.EXPECT().GetStats(mock.Anything, productID).Return(review.Stats{}, dbErr)

		result, err := svc.GetStats(ctx, productID)
		assert.Equal(t, review.Stats{}, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(ctx, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := review.NewService(repo, nil)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(core.ErrNotFound)

		err := svc.Delete(ctx, id)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}
