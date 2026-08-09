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

// This package shares test_product with every other product slice's postgres/
// package. It never resets or truncates -- every row it touches is seeded here
// with a fresh uuid.New() and cleaned up by name.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_product")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes product", func(t *testing.T) {
		id := seedProduct(t)
		repo := New(testPool)

		err := repo.Delete(context.Background(), id)
		require.NoError(t, err)

		var deletedAt *string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT deleted_at::text FROM products WHERE id = $1`, id).Scan(&deletedAt))
		require.NotNil(t, deletedAt, "Delete must set deleted_at")
	})

	t.Run("returns not found after delete", func(t *testing.T) {
		id := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, id))

		err := repo.Delete(ctx, id)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns not found for nonexistent product", func(t *testing.T) {
		repo := New(testPool)
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("Delete", func(t *testing.T) {
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

// seedProduct inserts a product with a fresh id and cleans it up on test
// completion -- this package never truncates a table it shares with every
// other product slice.
func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
	return id
}
