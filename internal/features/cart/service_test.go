package cart_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	cartMocks "github.com/residwi/go-api-project-template/mocks/cart"
)

func TestService_AddItem(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountItems(mock.Anything, cartID).
			Return(5, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 2})
		require.NoError(t, err)
	})

	t.Run("product not published", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Draft Item", Price: 500, Currency: "USD", Status: "draft"}, nil)

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("insufficient stock", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{Available: 1}, nil)

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 5})
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrInsufficientStock)
	})

	t.Run("cart full", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		maxItems := 3
		svc := cart.NewService(repo, nil, products, stock, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountItems(mock.Anything, cartID).
			Return(3, nil)

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("product not found", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).Return(nil, core.ErrNotFound)

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("stock check error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{}, errors.New("stock service unavailable"))

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(uuid.Nil, errors.New("db error"))

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})

	t.Run("count items error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		products := cartMocks.NewMockProductLookup(t)
		stock := cartMocks.NewMockStockChecker(t)
		svc := cart.NewService(repo, nil, products, stock, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Name: "Widget", Price: 1000, Currency: "USD", Status: "published"}, nil)
		stock.EXPECT().GetStock(mock.Anything, productID).
			Return(cart.StockInfo{Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountItems(mock.Anything, cartID).
			Return(0, errors.New("db error"))

		err := svc.AddItem(ctx, userID, cart.AddItemRequest{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})
}

func TestService_RemoveItem(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(nil)

		err := svc.RemoveItem(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(core.ErrNotFound)

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
	})
}

func TestService_UpdateQuantity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().UpdateItemQuantity(mock.Anything, cartID, productID, 3).Return(nil)

		err := svc.UpdateQuantity(ctx, userID, productID, cart.UpdateItemRequest{Quantity: 3})
		require.NoError(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.UpdateQuantity(ctx, userID, productID, cart.UpdateItemRequest{Quantity: 3})
		require.Error(t, err)
	})
}

func TestService_GetCart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		expected := &cart.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []cart.Item{},
		}

		repo.EXPECT().GetCart(mock.Anything, userID).Return(expected, nil)

		result, err := svc.GetCart(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, core.ErrNotFound)

		result, err := svc.GetCart(ctx, userID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_Clear(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().Clear(mock.Anything, cartID).Return(nil)

		err := svc.Clear(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		repo := cartMocks.NewMockRepository(t)
		svc := cart.NewService(repo, nil, nil, nil, 50)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.Clear(ctx, userID)
		require.Error(t, err)
	})
}
