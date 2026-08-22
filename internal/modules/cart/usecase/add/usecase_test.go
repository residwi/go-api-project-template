package add

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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Execute_RunsInsideTxRunner(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

	userID := uuid.New()
	productID := uuid.New()
	cartID := uuid.New()

	products.EXPECT().GetInfo(mock.Anything, productID).Return(&product.ProductInfo{
		ID:        productID,
		Name:      "Widget",
		Price:     money.New(1500, "USD"),
		Status:    "published",
		Available: 10,
	}, nil)
	repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
	repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).Return(0, false, nil)
	repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).Return(nil)

	err := cmd.Execute(context.Background(), userID, Params{
		ProductID: productID,
		Quantity:  2,
	})
	require.NoError(t, err)
}

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(5, false, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 2})
		require.NoError(t, err)
	})

	t.Run("product not published", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Draft Item", Price: money.New(500, "USD"), Status: "draft", Available: 10}, nil)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("insufficient stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 1}, nil)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 5})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("cart full", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		cmd := New(repo, testhelper.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(3, false, nil)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("cart full but bumping quantity of existing product succeeds", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		cmd := New(repo, testhelper.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(maxItems, true, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 2})
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(uuid.Nil, errors.New("db error"))

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})

	t.Run("cap check query error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.ProductInfo{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(0, false, errors.New("db error"))

		err := cmd.Execute(ctx, userID, Params{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})
}
