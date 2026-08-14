package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_ListItemsForUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Item{
			{ID: uuid.New(), ProductID: uuid.New()},
			{ID: uuid.New(), ProductID: uuid.New()},
		}

		repo.EXPECT().ListItemsForUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := New(repo).ListItemsForUser(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("empty wishlist", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().ListItemsForUser(mock.Anything, userID, cursor).Return([]domain.Item{}, nil)

		result, err := New(repo).ListItemsForUser(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().ListItemsForUser(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		_, err := New(repo).ListItemsForUser(ctx, userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
