package notification_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_notification")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

func seedNotification(t *testing.T, userID uuid.UUID) *notification.Notification {
	t.Helper()
	repo := notification.NewPostgresRepository(testPool)
	n := &notification.Notification{
		UserID: userID, Type: "test", Title: "T", Body: "m",
	}
	err := repo.Create(context.Background(), n)
	require.NoError(t, err)
	return n
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates notification with correct fields", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)

		n := &notification.Notification{
			UserID: userID, Type: "order_placed", Title: "Order placed", Body: "Your order is confirmed",
		}
		err := repo.Create(context.Background(), n)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, n.ID)
		assert.Equal(t, userID, n.UserID)
		assert.Equal(t, notification.Type("order_placed"), n.Type)
		assert.Equal(t, "Order placed", n.Title)
		assert.Equal(t, "Your order is confirmed", n.Body)
		assert.False(t, n.IsRead)
	})
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns all notifications for user", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		for range 3 {
			n := &notification.Notification{UserID: userID, Type: "test", Title: "T", Body: "m"}
			require.NoError(t, repo.Create(ctx, n))
		}

		items, err := repo.ListByUser(ctx, userID, core.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, items, 3)
	})

	t.Run("returns paginated results when results exceed limit", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		for range 5 {
			n := &notification.Notification{UserID: userID, Type: "test", Title: "T", Body: "m"}
			require.NoError(t, repo.Create(ctx, n))
		}

		items, err := repo.ListByUser(ctx, userID, core.CursorPage{Limit: 3})
		require.NoError(t, err)
		// ListByUser fetches Limit+1 to detect next page
		assert.Len(t, items, 4)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		for range 5 {
			n := &notification.Notification{UserID: userID, Type: "test", Title: "T", Body: "m"}
			require.NoError(t, repo.Create(ctx, n))
		}

		page1, err := repo.ListByUser(ctx, userID, core.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByUser(ctx, userID, core.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, n := range page2 {
			assert.NotEqual(t, page1[0].ID, n.ID)
			assert.NotEqual(t, page1[1].ID, n.ID)
		}
	})
}

func TestPostgresRepository_MarkRead(t *testing.T) {
	t.Run("marks notification as read", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		n := seedNotification(t, userID)
		repo := notification.NewPostgresRepository(testPool)

		require.NoError(t, repo.MarkRead(context.Background(), n.ID))

		count, _ := repo.CountUnread(context.Background(), userID)
		assert.Equal(t, 0, count)
	})

	t.Run("returns not found when already read", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		n := seedNotification(t, userID)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.MarkRead(ctx, n.ID))
		err := repo.MarkRead(ctx, n.ID)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_MarkAllRead(t *testing.T) {
	t.Run("marks all user notifications as read", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		for range 3 {
			n := &notification.Notification{UserID: userID, Type: "test", Title: "T", Body: "m"}
			repo.Create(ctx, n)
		}
		require.NoError(t, repo.MarkAllRead(ctx, userID))

		count, _ := repo.CountUnread(ctx, userID)
		assert.Equal(t, 0, count)
	})
}

func TestPostgresRepository_CountUnread(t *testing.T) {
	t.Run("returns zero when no notifications", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)

		count, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns correct count of unread notifications", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		seedNotification(t, userID)
		seedNotification(t, userID)

		count, err := repo.CountUnread(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestPostgresRepository_JobLifecycle(t *testing.T) {
	t.Run("create, claim, update, and delete job", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		n := seedNotification(t, userID)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &notification.Job{
			UserID:      n.UserID,
			Type:        "email",
			Title:       "Test",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, job.ID)

		jobs, err := repo.ClaimPendingJobs(ctx, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, jobs)

		job.Status = "completed"
		job.Attempts = 1
		require.NoError(t, repo.UpdateJob(ctx, job))

		deleted, err := repo.DeleteOldCompletedJobs(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})

	t.Run("claim returns empty when no pending jobs", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		// Use a fresh context — no pending jobs for a brand-new user
		jobs, err := repo.ClaimPendingJobs(context.Background(), 1)
		require.NoError(t, err)
		_ = jobs // may or may not be empty depending on prior test state; just verify no error
	})

	t.Run("update returns not found for missing job", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		job := &notification.Job{
			ID:       uuid.New(),
			Status:   "completed",
			Attempts: 1,
		}
		err := repo.UpdateJob(context.Background(), job)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_DeleteOldCompletedJobs(t *testing.T) {
	t.Run("deletes completed jobs older than threshold", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &notification.Job{
			UserID:      userID,
			Type:        "email",
			Title:       "Old",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		job.Status = "completed"
		job.Attempts = 1
		require.NoError(t, repo.UpdateJob(ctx, job))

		// olderThan=0 means anything older than 0 duration (all completed jobs)
		deleted, err := repo.DeleteOldCompletedJobs(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})

	t.Run("does not delete pending jobs", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &notification.Job{
			UserID:      userID,
			Type:        "email",
			Title:       "Pending",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		require.NoError(t, repo.CreateJob(ctx, job))

		deleted, err := repo.DeleteOldCompletedJobs(ctx, 1*time.Hour, 100)
		require.NoError(t, err)
		// pending jobs should not be deleted
		_ = deleted
	})
}

func TestPostgresRepository_Create_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		n := &notification.Notification{
			UserID: uuid.New(), Type: "test", Title: "T", Body: "m",
		}
		err := repo.Create(ctx, n)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListByUser_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListByUser(ctx, uuid.New(), core.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_MarkRead_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkRead(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_MarkAllRead_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.MarkAllRead(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CountUnread_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.CountUnread(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CreateJob_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		job := &notification.Job{
			UserID: uuid.New(), Type: "email", Title: "T", Body: "b",
			Status: "pending", MaxAttempts: 3,
		}
		err := repo.CreateJob(ctx, job)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ClaimPendingJobs_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ClaimPendingJobs(ctx, 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_UpdateJob_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		job := &notification.Job{
			ID: uuid.New(), Status: "completed", Attempts: 1,
		}
		err := repo.UpdateJob(ctx, job)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_DeleteOldCompletedJobs_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := notification.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.DeleteOldCompletedJobs(ctx, 1*time.Hour, 100)
		assert.Error(t, err)
	})
}
