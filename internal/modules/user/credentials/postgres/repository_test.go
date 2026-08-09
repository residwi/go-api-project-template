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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates user", func(t *testing.T) {
		repo := New(testPool)
		u := &domain.User{
			Email:        uuid.New().String() + "@example.com",
			PasswordHash: "hashed",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
		}

		err := repo.Create(context.Background(), u)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, u.ID)
		assert.False(t, u.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
		})
	})

	t.Run("returns conflict on duplicate email", func(t *testing.T) {
		existing := seedUser(t)
		repo := New(testPool)

		dup := &domain.User{
			Email:        existing.Email,
			PasswordHash: "hashed",
			FirstName:    "Jane",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
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

func TestPostgresRepository_GetByEmail(t *testing.T) {
	t.Run("returns user by email", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		got, err := repo.GetByEmail(context.Background(), u.Email)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByEmail(context.Background(), "nobody-"+uuid.New().String()+"@nowhere.example")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		u := &domain.User{
			Email:        uuid.New().String() + "@example.com",
			PasswordHash: "hashed",
			FirstName:    "Test",
			LastName:     "User",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("GetByEmail returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByEmail(ctx, "test@example.com")
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
