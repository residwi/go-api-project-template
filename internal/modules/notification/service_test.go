package notification

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}
		expected := []domain.Notification{
			{ID: uuid.New(), UserID: userID, Type: domain.TypeOrderPlaced, Title: "Order Placed"},
			{ID: uuid.New(), UserID: userID, Type: domain.TypeOrderShipped, Title: "Order Shipped"},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		var db database.DB
		var logger *slog.Logger
		result, err := New(repo, db, logger).List(t.Context(), userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()
		cursor := paging.CursorPage{Limit: 20}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		var db database.DB
		var logger *slog.Logger
		_, err := New(repo, db, logger).List(t.Context(), userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_CountUnread(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(5, nil)

		var db database.DB
		var logger *slog.Logger
		count, err := New(repo, db, logger).CountUnread(t.Context(), userID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(0, assert.AnError)

		var db database.DB
		var logger *slog.Logger
		_, err := New(repo, db, logger).CountUnread(t.Context(), userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_MarkRead(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID, id := uuid.New(), uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(nil)

		var db database.DB
		var logger *slog.Logger
		err := New(repo, db, logger).MarkRead(t.Context(), userID, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID, id := uuid.New(), uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, userID, id).Return(apperror.ErrNotFound)

		var db database.DB
		var logger *slog.Logger
		err := New(repo, db, logger).MarkRead(t.Context(), userID, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_MarkAllRead(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(nil)

		var db database.DB
		var logger *slog.Logger
		err := New(repo, db, logger).MarkAllRead(t.Context(), userID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)

		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(assert.AnError)

		var db database.DB
		var logger *slog.Logger
		err := New(repo, db, logger).MarkAllRead(t.Context(), userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
