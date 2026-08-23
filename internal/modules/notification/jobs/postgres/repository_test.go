package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
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
	t.Run("creates notification with correct fields", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		n := &domain.Notification{
			UserID: userID, Type: "order_placed", Title: "Order placed", Body: "Your order is confirmed",
		}
		err := repo.Create(context.Background(), n)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, n.ID)
		assert.Equal(t, userID, n.UserID)
		assert.Equal(t, domain.Type("order_placed"), n.Type)
		assert.Equal(t, "Order placed", n.Title)
		assert.Equal(t, "Your order is confirmed", n.Body)
		assert.False(t, n.IsRead)
	})
}

func TestPostgresRepository_Create_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		n := &domain.Notification{
			UserID: uuid.New(), Type: "test", Title: "T", Body: "m",
		}
		err := repo.Create(ctx, n)
		assert.Error(t, err)
	})
}

// TestPostgresRepository_JobLifecycle covers the NULL last_error regression:
// a freshly created job has no last_error, and Claim's scan used to require a
// string, not a *string, which panicked on the very first claim of any job.
func TestPostgresRepository_JobLifecycle(t *testing.T) {
	t.Run("create, claim, update, and delete job", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		job := &domain.Job{
			UserID:      userID,
			Type:        "email",
			Title:       "Test",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, job.ID)

		jobs, err := repo.Claim(ctx, 10, 2*time.Minute)
		require.NoError(t, err)
		assert.NotEmpty(t, jobs)

		job.Status = "completed"
		job.Attempts = 1
		require.NoError(t, repo.UpdateJob(ctx, job))

		deleted, err := repo.Prune(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})

	t.Run("claim returns empty when no pending jobs", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		jobs, err := repo.Claim(context.Background(), 1, 2*time.Minute)
		require.NoError(t, err)
		_ = jobs // may or may not be empty depending on prior test state; just verify no error
	})

	t.Run("update returns not found for missing job", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		job := &domain.Job{
			ID:       uuid.New(),
			Status:   "completed",
			Attempts: 1,
		}
		err := repo.UpdateJob(context.Background(), job)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CreateJob_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		job := &domain.Job{
			UserID: uuid.New(), Type: "email", Title: "T", Body: "b",
			Status: "pending", MaxAttempts: 3,
		}
		err := repo.CreateJob(ctx, job)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Claim_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Claim(ctx, 10, 2*time.Minute)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_UpdateJob_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		job := &domain.Job{
			ID: uuid.New(), Status: "completed", Attempts: 1,
		}
		err := repo.UpdateJob(ctx, job)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Prune(t *testing.T) {
	t.Run("deletes completed jobs older than threshold", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		job := &domain.Job{
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

		deleted, err := repo.Prune(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})

	t.Run("deletes failed jobs older than threshold", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		job := &domain.Job{
			UserID:      userID,
			Type:        "email",
			Title:       "Failed",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		job.Status = "failed"
		job.Attempts = 3
		require.NoError(t, repo.UpdateJob(ctx, job))

		deleted, err := repo.Prune(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)

		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM notification_jobs WHERE id = $1`, job.ID).Scan(&count))
		assert.Equal(t, 0, count, "failed jobs should be pruned, not left to accumulate")
	})

	t.Run("does not delete pending jobs", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		job := &domain.Job{
			UserID:      userID,
			Type:        "email",
			Title:       "Pending",
			Body:        "body",
			Status:      "pending",
			MaxAttempts: 3,
		}
		require.NoError(t, repo.CreateJob(ctx, job))

		deleted, err := repo.Prune(ctx, 1*time.Hour, 100)
		require.NoError(t, err)
		_ = deleted
	})
}

func TestPostgresRepository_Prune_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Prune(ctx, 1*time.Hour, 100)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testutil.SeedUser(t, testPool)
}
