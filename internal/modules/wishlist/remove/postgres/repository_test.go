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
	pool, cleanup := testhelper.MustStartPostgres("test_wishlist")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		wishlistID := seedWishlist(t, userID)
		seedItem(t, wishlistID, productID)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.RemoveItem(ctx, userID, productID))

		assert.False(t, itemExists(t, wishlistID, productID))
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		repo := New(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("RemoveItem", func(t *testing.T) {
		err := repo.RemoveItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedWishlist(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO wishlists (id, user_id) VALUES ($1, $2)`, id, userID)
	require.NoError(t, err)
	return id
}

func seedItem(t *testing.T, wishlistID, productID uuid.UUID) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO wishlist_items (wishlist_id, product_id) VALUES ($1, $2)`, wishlistID, productID)
	require.NoError(t, err)
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
