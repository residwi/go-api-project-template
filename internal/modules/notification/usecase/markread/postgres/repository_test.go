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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_notification")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_MarkRead(t *testing.T) {
	t.Run("marks notification as read", func(t *testing.T) {
		userID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(testPool)

		require.NoError(t, repo.MarkRead(context.Background(), userID, id))

		assert.True(t, isRead(t, id))
	})

	t.Run("returns not found for another user's notification", func(t *testing.T) {
		userID := seedUser(t)
		otherUserID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(testPool)

		err := repo.MarkRead(context.Background(), otherUserID, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found for a missing notification id", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		err := repo.MarkRead(context.Background(), userID, uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("is idempotent: re-marking an already-read notification succeeds", func(t *testing.T) {
		userID := seedUser(t)
		id := seedNotification(t, userID)
		repo := New(testPool)
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
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkRead(ctx, uuid.New(), uuid.New())
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
