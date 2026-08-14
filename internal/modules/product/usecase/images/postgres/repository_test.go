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
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
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

func TestPostgresRepository_Images(t *testing.T) {
	t.Run("add, list, and delete image", func(t *testing.T) {
		p := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		altText := "Test alt"
		img := &domain.Image{
			ProductID: p.ID,
			URL:       "https://example.com/image.jpg",
			AltText:   &altText,
			SortOrder: 0,
		}

		err := repo.AddImage(ctx, img)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, img.ID)

		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM product_images WHERE id = $1 AND url = $2`, img.ID, img.URL,
		).Scan(&count))
		assert.Equal(t, 1, count)

		err = repo.DeleteImage(ctx, img.ID)
		require.NoError(t, err)

		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM product_images WHERE id = $1`, img.ID,
		).Scan(&count))
		assert.Equal(t, 0, count)

		err = repo.DeleteImage(ctx, img.ID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("AddImage", func(t *testing.T) {
		img := &domain.Image{ProductID: uuid.New(), URL: "https://example.com/img.jpg", SortOrder: 0}
		err := repo.AddImage(cancelledCtx, img)
		assert.Error(t, err)
	})

	t.Run("DeleteImage", func(t *testing.T) {
		err := repo.DeleteImage(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

// seedProduct inserts a product with a fresh id and cleans it up on test
// completion -- this package never truncates a table it shares with every
// other product slice.
func seedProduct(t *testing.T) *domain.Product {
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
	return &domain.Product{ID: id}
}
