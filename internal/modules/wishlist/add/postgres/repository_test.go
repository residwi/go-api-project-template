package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_wishlist")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetOrCreate(t *testing.T) {
	t.Run("creates wishlist on first call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		wishlistID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, wishlistID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		first, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)

		second, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("adds product to wishlist", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		wishlistID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))

		assert.True(t, itemExists(t, wishlistID, productID))
	})

	t.Run("silently ignores duplicate (ON CONFLICT DO NOTHING)", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		wishlistID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetOrCreate", func(t *testing.T) {
		_, err := repo.GetOrCreate(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("AddItem", func(t *testing.T) {
		err := repo.AddItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Test Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

func itemExists(t *testing.T, wishlistID, productID uuid.UUID) bool {
	t.Helper()
	var exists bool
	err := testPool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM wishlist_items WHERE wishlist_id = $1 AND product_id = $2)`,
		wishlistID, productID,
	).Scan(&exists)
	require.NoError(t, err)
	return exists
}
