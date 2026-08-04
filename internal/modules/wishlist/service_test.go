package wishlist

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_GetWishlist(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []Item{
			{ID: uuid.New(), ProductID: uuid.New()},
			{ID: uuid.New(), ProductID: uuid.New()},
		}

		repo.EXPECT().GetItems(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := svc.GetWishlist(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("empty wishlist", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().GetItems(mock.Anything, userID, cursor).Return([]Item{}, nil)

		result, err := svc.GetWishlist(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestService_AddItem(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		wishlistID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(nil)

		err := svc.AddItem(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("get or create fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, assert.AnError)

		err := svc.AddItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_RemoveItem(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(nil)

		err := svc.RemoveItem(ctx, userID, productID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(apperror.ErrNotFound)

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().RemoveItem(mock.Anything, userID, productID).Return(assert.AnError)

		err := svc.RemoveItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_GetWishlist_RepoError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().GetItems(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		_, err := svc.GetWishlist(ctx, userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_AddItem_AddItemFails(t *testing.T) {
	t.Parallel()

	t.Run("add item repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		productID := uuid.New()
		wishlistID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(mock.Anything, wishlistID, productID).Return(assert.AnError)

		err := svc.AddItem(ctx, userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
