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
	pool, cleanup := testhelper.MustStartPostgres("test_category")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes category", func(t *testing.T) {
		id := seedCategory(t)
		repo := New(testPool)

		err := repo.Delete(context.Background(), id)
		require.NoError(t, err)

		var count int
		err = testPool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM categories WHERE id = $1`, id).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})
}

func seedCategory(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO categories (id, name, slug, sort_order, active)
		VALUES ($1, $2, $3, $4, $5)`,
		id, "Category-"+id.String()[:8], "slug-"+id.String(), 0, true,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM categories WHERE id = $1`, id)
	})
	return id
}
