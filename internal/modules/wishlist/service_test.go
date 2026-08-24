package wishlist

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_Add(t *testing.T) {
	t.Parallel()

	t.Run("creates the wishlist on first add", func(t *testing.T) {
		t.Parallel()

		userID, productID, wishlistID := uuid.New(), uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetOrCreate(t.Context(), userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(t.Context(), wishlistID, productID).Return(nil)

		require.NoError(t, New(repo).Add(t.Context(), userID, productID))
	})

	t.Run("get or create fails", func(t *testing.T) {
		t.Parallel()

		userID, productID := uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetOrCreate(t.Context(), userID).Return(uuid.Nil, assert.AnError)

		err := New(repo).Add(t.Context(), userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("add item repo error", func(t *testing.T) {
		t.Parallel()

		userID, productID, wishlistID := uuid.New(), uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetOrCreate(t.Context(), userID).Return(wishlistID, nil)
		repo.EXPECT().AddItem(t.Context(), wishlistID, productID).Return(assert.AnError)

		err := New(repo).Add(t.Context(), userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_Remove(t *testing.T) {
	t.Parallel()

	t.Run("removes the item", func(t *testing.T) {
		t.Parallel()

		userID, productID := uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().RemoveItem(t.Context(), userID, productID).Return(nil)

		require.NoError(t, New(repo).Remove(t.Context(), userID, productID))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		userID, productID := uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().RemoveItem(t.Context(), userID, productID).Return(apperror.ErrNotFound)

		err := New(repo).Remove(t.Context(), userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		userID, productID := uuid.New(), uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().RemoveItem(t.Context(), userID, productID).Return(assert.AnError)

		err := New(repo).Remove(t.Context(), userID, productID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("returns the user's items", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		items := []domain.Item{{ID: uuid.New(), ProductID: uuid.New()}}

		repo := NewMockRepository(t)
		repo.EXPECT().ListItemsForUser(t.Context(), userID, cursor).Return(items, nil)

		got, err := New(repo).List(t.Context(), userID, cursor)

		require.NoError(t, err)
		assert.Equal(t, items, got)
	})

	t.Run("empty wishlist", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo := NewMockRepository(t)
		repo.EXPECT().ListItemsForUser(t.Context(), userID, cursor).Return([]domain.Item{}, nil)

		got, err := New(repo).List(t.Context(), userID, cursor)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo := NewMockRepository(t)
		repo.EXPECT().ListItemsForUser(t.Context(), userID, cursor).Return(nil, assert.AnError)

		_, err := New(repo).List(t.Context(), userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
