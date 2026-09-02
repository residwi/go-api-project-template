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
	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
	"github.com/residwi/go-api-project-template/internal/features/money"
	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestService_Add(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(5, false, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := svc.Add(ctx, userID, productID, 2)
		require.NoError(t, err)
	})

	t.Run("product not published", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Draft Item", Price: money.New(500, "USD"), Status: "draft", Available: 10}, nil)

		err := svc.Add(ctx, userID, productID, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("insufficient stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 1}, nil)

		err := svc.Add(ctx, userID, productID, 5)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("cart full", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		svc := New(repo, testutil.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(3, false, nil)

		err := svc.Add(ctx, userID, productID, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("cart full but bumping quantity of existing product succeeds", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		maxItems := 3
		svc := New(repo, testutil.FakeTxRunner{}, products, maxItems)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(maxItems, true, nil)
		repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).
			Return(nil)

		err := svc.Add(ctx, userID, productID, 2)
		require.NoError(t, err)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, errs.ErrNotFound)

		err := svc.Add(ctx, userID, productID, 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(uuid.Nil, errors.New("db error"))

		err := svc.Add(ctx, userID, productID, 1)
		require.Error(t, err)
	})

	t.Run("cap check query error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		svc := New(repo, testutil.FakeTxRunner{}, products, 50)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).
			Return(cartID, nil)
		repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).
			Return(0, false, errors.New("db error"))

		err := svc.Add(ctx, userID, productID, 1)
		require.Error(t, err)
	})
}

func TestService_Add_RunsInsideTxRunner(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	svc := New(repo, testutil.FakeTxRunner{}, products, 50)

	userID := uuid.New()
	productID := uuid.New()
	cartID := uuid.New()

	products.EXPECT().GetInfo(mock.Anything, productID).Return(&product.Info{
		ID:        productID,
		Name:      "Widget",
		Price:     money.New(1500, "USD"),
		Status:    "published",
		Available: 10,
	}, nil)
	repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
	repo.EXPECT().CountAndHasItem(mock.Anything, cartID, productID).Return(0, false, nil)
	repo.EXPECT().AddItem(mock.Anything, cartID, productID, 2).Return(nil)

	err := svc.Add(context.Background(), userID, productID, 2)
	require.NoError(t, err)
}

func TestService_Remove(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(nil)

		err := svc.Remove(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(errs.ErrNotFound)

		err := svc.Remove(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.Remove(ctx, userID, productID)
		require.Error(t, err)
	})
}

func TestService_UpdateQuantity(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().UpdateItemQuantity(mock.Anything, cartID, productID, 3).Return(nil)

		err := svc.UpdateQuantity(ctx, userID, productID, 3)
		require.NoError(t, err)
	})

	t.Run("rejects quantity above available stock", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "published", Available: 2}, nil)

		err := svc.UpdateQuantity(ctx, userID, productID, 5)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})

	t.Run("rejects unpublished product", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "draft", Available: 100}, nil)

		err := svc.UpdateQuantity(ctx, userID, productID, 1)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("product lookup error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, errors.New("db error"))

		err := svc.UpdateQuantity(ctx, userID, productID, 3)
		require.Error(t, err)
	})

	t.Run("get or create error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		products := NewMockProductLookup(t)
		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&product.Info{ID: productID, Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db error"))

		err := svc.UpdateQuantity(ctx, userID, productID, 3)
		require.Error(t, err)
	})
}

func TestService_Get(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()
		expected := &domain.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []domain.Item{},
		}

		repo.EXPECT().GetCart(mock.Anything, userID).Return(expected, nil)

		result, err := svc.Get(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, errs.ErrNotFound)

		result, err := svc.Get(ctx, userID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_Get_FlagsUnavailableLines(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	var tx database.TxRunner
	svc := New(repo, tx, products, 0)

	userID := uuid.New()
	liveID, goneID := uuid.New(), uuid.New()

	repo.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
		UserID: userID,
		Items: []domain.Item{
			{ProductID: liveID, Quantity: 2},
			{ProductID: goneID, Quantity: 1},
		},
	}, nil)

	// A soft-deleted product still comes back carrying its status: cart decides how
	// to show it, not product.
	products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{liveID, goneID}).
		Return(map[uuid.UUID]product.Info{
			liveID: {ID: liveID, Name: "Widget", Price: money.New(1500, "USD"), Status: "published", Available: 5},
			goneID: {ID: goneID, Name: "Gone", Price: money.New(900, "USD"), Status: "archived", Available: 0},
		}, nil)

	c, err := svc.Get(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 2, "an unsellable line must be shown, not hidden")
	assert.Equal(t, "published", c.Items[0].Product.Status)
	assert.Equal(t, "archived", c.Items[1].Product.Status)
}

func TestService_Get_MissingProductBecomesUnavailable(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	var tx database.TxRunner
	svc := New(repo, tx, products, 0)

	userID := uuid.New()
	missingID := uuid.New()

	repo.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
		UserID: userID,
		Items: []domain.Item{
			{ProductID: missingID, Quantity: 1},
		},
	}, nil)

	// Absent from the map, not merely carrying a terminal status: the line must
	// still render non-nil so no consumer of Item.Product has to nil-check it.
	products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{missingID}).
		Return(map[uuid.UUID]product.Info{}, nil)

	c, err := svc.Get(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 1)
	require.NotNil(t, c.Items[0].Product)
	assert.Equal(t, "unavailable", c.Items[0].Product.Status)
}

func TestService_Snapshot(t *testing.T) {
	t.Parallel()

	t.Run("flattens each line's product into the snapshot item", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		cartID := uuid.New()
		productID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			ID:     cartID,
			UserID: userID,
			Items:  []domain.Item{{ProductID: productID, Quantity: 3}},
		}, nil)

		products := NewMockProductLookup(t)
		products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]product.Info{
				productID: {Name: "Widget", Price: money.New(1500, "IDR"), Status: "published"},
			}, nil)

		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		got, err := svc.Snapshot(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, &Snapshot{
			ID: cartID,
			Items: []Item{{
				ProductID: productID,
				Quantity:  3,
				Name:      "Widget",
				Price:     money.New(1500, "IDR"),
				Status:    "published",
			}},
		}, got)
	})

	// Get's own fallback for a product it can't resolve is a non-nil Product
	// carrying "unavailable" -- never a nil Product -- so that is the value
	// Snapshot's flattening loop actually sees.
	t.Run("a product Get could not resolve flattens to its unavailable status", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		productID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			Items: []domain.Item{{ProductID: productID, Quantity: 1}},
		}, nil)

		products := NewMockProductLookup(t)
		products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]product.Info{}, nil)

		var tx database.TxRunner
		svc := New(repo, tx, products, 0)

		got, err := svc.Snapshot(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, Item{ProductID: productID, Quantity: 1, Status: "unavailable"}, got.Items[0])
	})
}

func TestService_Clear(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(nil)

		err := svc.Clear(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(errors.New("db error"))

		err := svc.Clear(ctx, userID)
		require.Error(t, err)
	})
}

// Lock discards the cart id GetCartForLock returns and passes through only
// the error, so this is the only place that proves both halves of that:
// a found cart returns nil, and ErrNotFound reaches order.Cart's
// caller unchanged.
func TestService_Lock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		userID := uuid.New()
		repo.EXPECT().GetCartForLock(mock.Anything, userID).Return(uuid.New(), nil)

		err := svc.Lock(context.Background(), userID)
		require.NoError(t, err)
	})

	t.Run("not found propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		var products ProductLookup
		svc := New(repo, tx, products, 0)

		userID := uuid.New()
		repo.EXPECT().GetCartForLock(mock.Anything, userID).Return(uuid.Nil, errs.ErrNotFound)

		err := svc.Lock(context.Background(), userID)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}
