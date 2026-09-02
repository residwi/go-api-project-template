package review

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/review/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()
		orderID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, orderID, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(rv *domain.Review) bool {
			return rv.UserID == userID &&
				rv.ProductID == productID &&
				rv.OrderID == orderID &&
				rv.Rating == 5 &&
				rv.Title == "Great product" &&
				rv.Body == "Really loved it" &&
				rv.Status == "published"
		})).Return(nil)

		result, err := svc.Create(t.Context(), userID, productID, orderID, 5, "Great product", "Really loved it")
		require.NoError(t, err)
		assert.Equal(t, &domain.Review{
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
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, nil)

		result, err := svc.Create(t.Context(), userID, productID, uuid.New(), 4, "Good", "")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("purchase verifier error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()

		verifyErr := errors.New("purchase check failed")
		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, verifyErr)

		result, err := svc.Create(t.Context(), userID, productID, uuid.New(), 5, "Great", "")
		assert.Nil(t, result)
		assert.ErrorIs(t, err, verifyErr)
	})

	t.Run("HasUserReviewed error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, dbErr)

		result, err := svc.Create(t.Context(), userID, productID, uuid.New(), 4, "Good", "")
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("repo create error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		createErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Review")).Return(createErr)

		result, err := svc.Create(t.Context(), userID, productID, uuid.New(), 3, "OK", "")
		assert.Nil(t, result)
		assert.ErrorIs(t, err, createErr)
	})

	t.Run("already reviewed", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		svc := New(repo, purchase)

		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(true, nil)

		result, err := svc.Create(t.Context(), userID, productID, uuid.New(), 3, "OK", "")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

// The argument order is userID, orderID, productID -- this pins it, since
// there is no longer a named struct to catch a positional swap at compile
// time. All three ids are distinct: reusing one would let a swapped call
// still pass by accident.
func TestService_Create_PassesArgumentsInOrder(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	purchase := NewMockPurchaseVerifier(t)
	svc := New(repo, purchase)

	userID := uuid.New()
	productID := uuid.New()
	orderID := uuid.New()

	purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, orderID, productID).Return(false, nil)

	_, err := svc.Create(t.Context(), userID, productID, orderID, 5, "Great", "Worked well")
	require.ErrorIs(t, err, errs.ErrBadRequest, "a non-delivered purchase must be rejected")
}

func TestService_ListByProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Review{
			{ID: uuid.New(), ProductID: productID, Rating: 5, Title: "Great"},
			{ID: uuid.New(), ProductID: productID, Rating: 4, Title: "Good"},
		}

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(expected, nil)

		result, err := svc.ListByProduct(t.Context(), productID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		productID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		dbErr := errors.New("query failed")

		repo.EXPECT().ListByProduct(mock.Anything, productID, cursor).Return(nil, dbErr)

		result, err := svc.ListByProduct(t.Context(), productID, cursor)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetStats(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		productID := uuid.New()
		expected := domain.Stats{AverageRating: 4.5, TotalReviews: 10}

		repo.EXPECT().GetStats(mock.Anything, productID).Return(expected, nil)

		result, err := svc.GetStats(t.Context(), productID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		productID := uuid.New()
		dbErr := errors.New("stats query failed")

		repo.EXPECT().GetStats(mock.Anything, productID).Return(domain.Stats{}, dbErr)

		result, err := svc.GetStats(t.Context(), productID)
		assert.Equal(t, domain.Stats{}, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(t.Context(), id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var purchase PurchaseVerifier
		svc := New(repo, purchase)

		id := uuid.New()

		repo.EXPECT().Delete(mock.Anything, id).Return(errs.ErrNotFound)

		err := svc.Delete(t.Context(), id)
		require.ErrorIs(t, err, errs.ErrNotFound)
	})
}
