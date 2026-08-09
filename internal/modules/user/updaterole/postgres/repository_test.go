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

// CountAdmins is a global aggregate over a database this package shares with
// every other user slice's postgres/ package, so a sibling package's own
// admin-seeding subtest can run concurrently and bump the count between this
// subtest's own before/after reads. Asserting a lower bound rather than exact
// equality keeps the assertion true regardless of what a concurrent sibling
// does.
func TestPostgresRepository_CountAdmins(t *testing.T) {
	t.Run("returns a non-negative count", func(t *testing.T) {
		repo := New(testPool)

		count, err := repo.CountAdmins(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("increases after seeding an active admin", func(t *testing.T) {
		repo := New(testPool)
		ctx := context.Background()

		before, err := repo.CountAdmins(ctx)
		require.NoError(t, err)

		testhelper.SeedUserWith(t, testPool, testhelper.SeedUserOpts{
			FirstName: "Admin",
			LastName:  "User",
			Role:      "admin",
		})

		after, err := repo.CountAdmins(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, after, before+1)
	})
}

func TestPostgresRepository_IncrementTokenVersion(t *testing.T) {
	t.Run("increments token version", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)
		ctx := context.Background()

		err := repo.IncrementTokenVersion(ctx, u.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.TokenVersion+1, got.TokenVersion)
	})

	t.Run("returns not found for missing user", func(t *testing.T) {
		repo := New(testPool)

		err := repo.IncrementTokenVersion(context.Background(), uuid.New())
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

	t.Run("CountAdmins returns error on cancelled context", func(t *testing.T) {
		_, err := repo.CountAdmins(ctx)
		require.Error(t, err)
	})

	t.Run("IncrementTokenVersion returns error on cancelled context", func(t *testing.T) {
		err := repo.IncrementTokenVersion(ctx, uuid.New())
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
