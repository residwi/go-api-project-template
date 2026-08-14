package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_cart")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetOrCreate(t *testing.T) {
	t.Run("creates cart on first call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		cartID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, cartID)
		cleanupCart(t, cartID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		first, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		cleanupCart(t, first)

		second, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("accumulates quantity on duplicate insert", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		cleanupCart(t, cartID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 2))
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 3))

		// Same predicate CountItems used before it was dropped as dead code: no
		// production caller ever read the count it returned.
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM cart_items WHERE cart_id = $1`, cartID).Scan(&count))
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_CountAndHasItem(t *testing.T) {
	t.Run("returns zero count and false when the product is not in the cart", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		cleanupCart(t, cartID)

		count, hasProduct, err := repo.CountAndHasItem(ctx, cartID, productID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.False(t, hasProduct)
	})

	t.Run("returns the distinct count and true when the product is in the cart", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		otherID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		cleanupCart(t, cartID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.AddItem(ctx, cartID, otherID, 1))

		count, hasProduct, err := repo.CountAndHasItem(ctx, cartID, productID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.True(t, hasProduct)
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
		err := repo.AddItem(cancelledCtx, uuid.New(), uuid.New(), 1)
		assert.Error(t, err)
	})

	t.Run("CountAndHasItem", func(t *testing.T) {
		_, _, err := repo.CountAndHasItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}

// The application only soft-deletes users, so the old ON DELETE CASCADE was
// unreachable configuration implying a cart cleanup nothing performed.
func TestUserHardDeleteIsRestricted(t *testing.T) {
	ctx := context.Background()

	userID := seedUser(t)
	repo := New(testPool)
	cartID, err := repo.GetOrCreate(ctx, userID)
	require.NoError(t, err)
	cleanupCart(t, cartID)

	_, err = testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.Error(t, err, "hard-deleting a user with a cart must be refused")
	assert.True(t, database.IsForeignKeyViolation(err),
		"expected a foreign key violation, got: %v", err)

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM carts WHERE user_id = $1`, userID).Scan(&count))
	assert.Equal(t, 1, count, "cart must survive the refused delete")
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

// cleanupCart deletes the cart row; cart_items cascades from it. Registered
// after seedUser so it runs first in t.Cleanup's LIFO order -- the user's own
// row can only be deleted once its cart is gone.
func cleanupCart(t *testing.T, cartID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM carts WHERE id = $1`, cartID) })
}
