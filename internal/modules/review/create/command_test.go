package create

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		cmd := New(repo, purchase)

		ctx := context.Background()
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

		p := Params{
			OrderID: orderID,
			Rating:  5,
			Title:   "Great product",
			Body:    "Really loved it",
		}

		result, err := cmd.Execute(ctx, userID, productID, p)
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
		cmd := New(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, nil)

		p := Params{
			OrderID: uuid.New(),
			Rating:  4,
			Title:   "Good",
		}

		result, err := cmd.Execute(ctx, userID, productID, p)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("purchase verifier error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		cmd := New(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		verifyErr := errors.New("purchase check failed")
		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(false, verifyErr)

		p := Params{OrderID: uuid.New(), Rating: 5, Title: "Great"}
		result, err := cmd.Execute(ctx, userID, productID, p)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, verifyErr)
	})

	t.Run("HasUserReviewed error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		cmd := New(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, dbErr)

		p := Params{OrderID: uuid.New(), Rating: 4, Title: "Good"}
		result, err := cmd.Execute(ctx, userID, productID, p)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("repo create error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		cmd := New(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(false, nil)
		createErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Review")).Return(createErr)

		p := Params{OrderID: uuid.New(), Rating: 3, Title: "OK"}
		result, err := cmd.Execute(ctx, userID, productID, p)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, createErr)
	})

	t.Run("already reviewed", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		purchase := NewMockPurchaseVerifier(t)
		cmd := New(repo, purchase)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		purchase.EXPECT().HasDeliveredOrder(mock.Anything, userID, mock.Anything, productID).Return(true, nil)
		repo.EXPECT().HasUserReviewed(mock.Anything, userID, productID).Return(true, nil)

		p := Params{
			OrderID: uuid.New(),
			Rating:  3,
			Title:   "OK",
		}

		result, err := cmd.Execute(ctx, userID, productID, p)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

// The argument order is userID, orderID, productID -- this pins it, since
// there is no longer a named struct to catch a positional swap at compile
// time. All three ids are distinct: reusing one would let a swapped call
// still pass by accident.
func TestCommand_Execute_PassesArgumentsInOrder(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	verifier := NewMockPurchaseVerifier(t)
	cmd := New(repo, verifier)

	userID := uuid.New()
	productID := uuid.New()
	orderID := uuid.New()

	verifier.EXPECT().HasDeliveredOrder(mock.Anything, userID, orderID, productID).Return(false, nil)

	_, err := cmd.Execute(context.Background(), userID, productID, Params{
		OrderID: orderID,
		Rating:  5,
		Title:   "Great",
		Body:    "Worked well",
	})
	require.ErrorIs(t, err, apperror.ErrBadRequest, "a non-delivered purchase must be rejected")
}
