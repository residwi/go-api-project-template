package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_notification")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns all notifications for user", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
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
		repo := New(testPool)
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
		repo := New(testPool)
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
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListByUser(ctx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CountUnread(t *testing.T) {
	t.Run("returns zero when no notifications", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		count, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns correct count of unread notifications", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		seedNotification(t, userID)
		seedNotification(t, userID)

		count, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestPostgresRepository_CountUnread_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.CountUnread(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

// seedNotification inserts a row directly: this slice's Repository has no
// Create -- that write path lives in jobs/postgres, which owns the queue
// that produces notifications.
func seedNotification(t *testing.T, userID uuid.UUID) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO notifications (user_id, type, title, body, is_read)
		VALUES ($1, 'test', 'T', 'm', false)`,
		userID,
	)
	require.NoError(t, err)
}
