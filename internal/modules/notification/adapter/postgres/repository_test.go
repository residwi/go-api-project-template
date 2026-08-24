package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_notification")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("inserts a notification and returns its id and timestamp", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		n := &domain.Notification{UserID: userID, Title: "Order Placed", Body: "Your order has been placed."}

		require.NoError(t, repo.Create(context.Background(), n))
		assert.NotEqual(t, uuid.Nil, n.ID)
		assert.False(t, n.CreatedAt.IsZero())
	})
}

func TestPostgresRepository_Get(t *testing.T) {
	t.Run("returns the notification by id", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		n := &domain.Notification{UserID: userID, Title: "Order Placed", Body: "Your order has been placed."}
		require.NoError(t, repo.Create(context.Background(), n))

		got, err := repo.Get(context.Background(), n.ID)
		require.NoError(t, err)
		assert.Equal(t, *n, got)
	})

	t.Run("returns not found for a missing id", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.Get(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns all notifications for user", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		for range 3 {
			seedNotification(t, userID)
		}

		items, err := repo.ListByUser(ctx, userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, items, 3)
	})

	t.Run("returns paginated results when results exceed limit", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		for range 5 {
			seedNotification(t, userID)
		}

		items, err := repo.ListByUser(ctx, userID, paging.CursorPage{Limit: 3})
		require.NoError(t, err)
		// ListByUser fetches Limit+1 to detect next page
		assert.Len(t, items, 4)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		for range 5 {
			seedNotification(t, userID)
		}

		page1, err := repo.ListByUser(ctx, userID, paging.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByUser(ctx, userID, paging.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, n := range page2 {
			assert.NotEqual(t, page1[0].ID, n.ID)
			assert.NotEqual(t, page1[1].ID, n.ID)
		}
	})
}

func TestPostgresRepository_ListByUser_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListByUser(ctx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CountUnread(t *testing.T) {
	t.Run("returns zero when no notifications", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		count, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns correct count of unread notifications", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		seedNotification(t, userID)
		seedNotification(t, userID)

		count, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestPostgresRepository_CountUnread_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.CountUnread(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_MarkRead(t *testing.T) {
	t.Run("marks notification as read", func(t *testing.T) {
		userID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(database.DB{Primary: testPool})

		require.NoError(t, repo.MarkRead(context.Background(), userID, id))

		assert.True(t, isRead(t, id))
	})

	t.Run("returns not found for another user's notification", func(t *testing.T) {
		userID := seedUser(t)
		otherUserID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(database.DB{Primary: testPool})

		err := repo.MarkRead(context.Background(), otherUserID, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found for a missing notification id", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		err := repo.MarkRead(context.Background(), userID, uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("is idempotent: re-marking an already-read notification succeeds", func(t *testing.T) {
		userID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		require.NoError(t, repo.MarkRead(ctx, userID, id))
		// Marking the same owned notification read again must succeed (no 404),
		// because the notification demonstrably exists and belongs to the user.
		require.NoError(t, repo.MarkRead(ctx, userID, id))

		assert.True(t, isRead(t, id))
	})
}

func TestPostgresRepository_MarkRead_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkRead(ctx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_MarkAllRead(t *testing.T) {
	t.Run("marks all user notifications as read", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		for range 3 {
			seedNotification(t, userID)
		}
		require.NoError(t, repo.MarkAllRead(ctx, userID))

		assert.Equal(t, 0, unreadCount(t, userID))
	})
}

func TestPostgresRepository_MarkAllRead_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkAllRead(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testutil.SeedUser(t, testPool)
}

func seedNotification(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO notifications (id, user_id, type, title, body, is_read)
		VALUES ($1, $2, 'test', 'T', 'm', false)`,
		id, userID,
	)
	require.NoError(t, err)
	return id
}

func isRead(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	var read bool
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT is_read FROM notifications WHERE id = $1`, id).Scan(&read))
	return read
}

func unreadCount(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID).Scan(&count))
	return count
}
