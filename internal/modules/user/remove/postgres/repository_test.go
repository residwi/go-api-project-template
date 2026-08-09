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
		id := testhelper.SeedUser(t, testPool)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, id, got.ID)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes user", func(t *testing.T) {
		id := testhelper.SeedUser(t, testPool)
		repo := New(testPool)

		err := repo.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("GetByID returns not found after delete", func(t *testing.T) {
		id := testhelper.SeedUser(t, testPool)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, id))

		_, err := repo.GetByID(ctx, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found for nonexistent user", func(t *testing.T) {
		repo := New(testPool)
		err := repo.Delete(context.Background(), uuid.New())
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

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("CountAdmins returns error on cancelled context", func(t *testing.T) {
		_, err := repo.CountAdmins(ctx)
		require.Error(t, err)
	})
}
