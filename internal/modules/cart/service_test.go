package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestService_AddItem_RunsInsideTxRunner(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

	userID := uuid.New()
	productID := uuid.New()
	cartID := uuid.New()

	products.EXPECT().GetInfo(mock.Anything, productID).Return(&productcontract.Product{
		ID:        productID,
		Name:      "Widget",
		Price:     money.New(1500, "USD"),
		Status:    "published",
		Available: 10,
	}, nil)
	repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
	repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).Return(0, false, nil)
	repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).Return(nil)

	err := svc.AddItem(context.Background(), userID, AddItemParams{
		ProductID: productID,
		Quantity:  2,
	})
	require.NoError(t, err)
}

func TestService_AddItem(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(5, false, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 2})
		require.NoError(t, err)
	})

	t.Run("product not published", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Draft Item", Price: money.New(500, "USD"), Status: "draft", Available: 10}, nil)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("insufficient stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 1}, nil)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 5})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("cart full", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(3, false, nil)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("cart full but bumping quantity of existing product succeeds", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(maxItems, true, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 2})
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(uuid.Nil, errors.New("db error"))

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})

	t.Run("cap check query error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(0, false, errors.New("db error"))

		err := svc.AddItem(ctx, userID, AddItemParams{ProductID: productID, Quantity: 1})
		require.Error(t, err)
	})
}

func TestService_RemoveItem(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(apperror.ErrNotFound)

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
	})
}

func TestService_UpdateQuantity(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().UpdateItemQuantity(mock.Anything, cartID, productID, 3).Return(nil)

		err := svc.UpdateQuantity(ctx, userID, productID, UpdateQuantityParams{Quantity: 3})
		require.NoError(t, err)
	})

	t.Run("rejects quantity above available stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Status: "published", Available: 2}, nil)

		err := svc.UpdateQuantity(ctx, userID, productID, UpdateQuantityParams{Quantity: 5})
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("rejects unpublished product", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Status: "draft", Available: 100}, nil)

		err := svc.UpdateQuantity(ctx, userID, productID, UpdateQuantityParams{Quantity: 1})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("product lookup error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, errors.New("db error"))

		err := svc.UpdateQuantity(ctx, userID, productID, UpdateQuantityParams{Quantity: 3})
		require.Error(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.UpdateQuantity(ctx, userID, productID, UpdateQuantityParams{Quantity: 3})
		require.Error(t, err)
	})
}

func TestService_GetCart(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()
		expected := &Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []Item{},
		}

		repo.EXPECT().GetCart(mock.Anything, userID).Return(expected, nil)

		result, err := svc.GetCart(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		result, err := svc.GetCart(ctx, userID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_GetCart_FlagsUnavailableLines(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

	userID := uuid.New()
	liveID, goneID := uuid.New(), uuid.New()

	repo.EXPECT().GetCart(mock.Anything, userID).Return(&Cart{
		UserID: userID,
		Items: []Item{
			{ProductID: liveID, Quantity: 2},
			{ProductID: goneID, Quantity: 1},
		},
	}, nil)

	// A soft-deleted product still comes back carrying its status: cart decides how
	// to show it, not product.
	products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{liveID, goneID}).
		Return(map[uuid.UUID]productcontract.Product{
			liveID: {ID: liveID, Name: "Widget", Price: money.New(1500, "USD"), Status: "published", Available: 5},
			goneID: {ID: goneID, Name: "Gone", Price: money.New(900, "USD"), Status: "archived", Available: 0},
		}, nil)

	c, err := svc.GetCart(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 2, "an unsellable line must be shown, not hidden")
	assert.Equal(t, "published", c.Items[0].Product.Status)
	assert.Equal(t, "archived", c.Items[1].Product.Status)
}

func TestService_GetCart_MissingProductBecomesUnavailable(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	svc := NewService(repo, testhelper.FakeTxRunner{}, products, 50)

	userID := uuid.New()
	missingID := uuid.New()

	repo.EXPECT().GetCart(mock.Anything, userID).Return(&Cart{
		UserID: userID,
		Items: []Item{
			{ProductID: missingID, Quantity: 1},
		},
	}, nil)

	// Absent from the map, not merely carrying a terminal status: the line must
	// still render non-nil so no consumer of Item.Product has to nil-check it.
	products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{missingID}).
		Return(map[uuid.UUID]productcontract.Product{}, nil)

	c, err := svc.GetCart(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 1)
	require.NotNil(t, c.Items[0].Product)
	assert.Equal(t, "unavailable", c.Items[0].Product.Status)
}

func TestService_Clear(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(nil)

		err := svc.Clear(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, nil, 50)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(errors.New("db error"))

		err := svc.Clear(ctx, userID)
		require.Error(t, err)
	})
}
