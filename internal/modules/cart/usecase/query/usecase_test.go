package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/money"
)

func TestReader_GetCart(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo, nil)

		ctx := context.Background()
		userID := uuid.New()
		expected := &domain.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []domain.Item{},
		}

		repo.EXPECT().GetCart(mock.Anything, userID).Return(expected, nil)

		result, err := r.GetCart(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo, nil)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		result, err := r.GetCart(ctx, userID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestReader_GetCart_FlagsUnavailableLines(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	r := New(repo, products)

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
		Return(map[uuid.UUID]productcontract.Product{
			liveID: {ID: liveID, Name: "Widget", Price: money.New(1500, "USD"), Status: "published", Available: 5},
			goneID: {ID: goneID, Name: "Gone", Price: money.New(900, "USD"), Status: "archived", Available: 0},
		}, nil)

	c, err := r.GetCart(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 2, "an unsellable line must be shown, not hidden")
	assert.Equal(t, "published", c.Items[0].Product.Status)
	assert.Equal(t, "archived", c.Items[1].Product.Status)
}

func TestReader_GetCart_MissingProductBecomesUnavailable(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	r := New(repo, products)

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
		Return(map[uuid.UUID]productcontract.Product{}, nil)

	c, err := r.GetCart(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, c.Items, 1)
	require.NotNil(t, c.Items[0].Product)
	assert.Equal(t, "unavailable", c.Items[0].Product.Status)
}

func TestReader_Snapshot(t *testing.T) {
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
			Return(map[uuid.UUID]productcontract.Product{
				productID: {Name: "Widget", Price: money.New(1500, "IDR"), Status: "published"},
			}, nil)

		r := New(repo, products)

		got, err := r.Snapshot(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, &cartcontract.Cart{
			ID: cartID,
			Items: []cartcontract.CartItem{{
				ProductID: productID,
				Quantity:  3,
				Name:      "Widget",
				Price:     money.New(1500, "IDR"),
				Status:    "published",
			}},
		}, got)
	})

	// GetCart's own fallback for a product it can't resolve is a non-nil
	// Product carrying "unavailable" -- never a nil Product -- so that is the
	// value Snapshot's flattening loop actually sees.
	t.Run("a product GetCart could not resolve flattens to its unavailable status", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		productID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			Items: []domain.Item{{ProductID: productID, Quantity: 1}},
		}, nil)

		products := NewMockProductLookup(t)
		products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{productID}).
			Return(map[uuid.UUID]productcontract.Product{}, nil)

		r := New(repo, products)

		got, err := r.Snapshot(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, cartcontract.CartItem{ProductID: productID, Quantity: 1, Status: "unavailable"}, got.Items[0])
	})
}
