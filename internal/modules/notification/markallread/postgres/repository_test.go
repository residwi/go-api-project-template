package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_notification")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_MarkAllRead(t *testing.T) {
	t.Run("marks all user notifications as read", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
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
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkAllRead(ctx, uuid.New())
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

func unreadCount(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID).Scan(&count))
	return count
}
