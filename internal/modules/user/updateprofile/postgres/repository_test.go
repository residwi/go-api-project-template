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
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// This package shares test_user with every other user slice's postgres/
// package; see the registry comment in internal/testhelper. It never resets
// or truncates -- every row it touches is seeded here with a fresh uuid.New()
// and cleaned up by name.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_user")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns user", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates user fields", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		u.FirstName = "Updated"
		u.LastName = "Name"
		u.Active = false
		err := repo.Update(context.Background(), u)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated", got.FirstName)
		assert.Equal(t, "Name", got.LastName)
		assert.False(t, got.Active)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		u := &domain.User{
			ID:        uuid.New(),
			FirstName: "Ghost",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.Update(context.Background(), u)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("Update returns error on cancelled context", func(t *testing.T) {
		u := &domain.User{
			ID:        uuid.New(),
			FirstName: "Test",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.Update(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})
}

func seedUser(t *testing.T) *domain.User {
	t.Helper()
	id := testhelper.SeedUser(t, testPool)

	repo := New(testPool)
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}
