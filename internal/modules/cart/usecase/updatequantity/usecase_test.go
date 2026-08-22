package updatequantity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/money"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, products)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().UpdateItemQuantity(mock.Anything, cartID, productID, 3).Return(nil)

		err := cmd.Execute(ctx, userID, productID, Params{Quantity: 3})
		require.NoError(t, err)
	})

	t.Run("rejects quantity above available stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, products)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "published", Available: 2}, nil)

		err := cmd.Execute(ctx, userID, productID, Params{Quantity: 5})
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("rejects unpublished product", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, products)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "draft", Available: 100}, nil)

		err := cmd.Execute(ctx, userID, productID, Params{Quantity: 1})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("product lookup error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, products)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, errors.New("db error"))

		err := cmd.Execute(ctx, userID, productID, Params{Quantity: 3})
		require.Error(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, products)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := cmd.Execute(ctx, userID, productID, Params{Quantity: 3})
		require.Error(t, err)
	})
}
