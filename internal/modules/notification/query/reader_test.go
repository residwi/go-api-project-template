package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestReader_ListByUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Notification{
			{ID: uuid.New(), UserID: userID, Type: domain.TypeOrderPlaced, Title: "Order Placed"},
			{ID: uuid.New(), UserID: userID, Type: domain.TypeOrderShipped, Title: "Order Shipped"},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := reader.ListByUser(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		_, err := reader.ListByUser(ctx, userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestReader_CountUnread(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(5, nil)

		count, err := reader.CountUnread(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		reader := New(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(0, assert.AnError)

		_, err := reader.CountUnread(ctx, userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
