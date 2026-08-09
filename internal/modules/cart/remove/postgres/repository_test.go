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

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		cleanupCart(t, cartID)
		require.NoError(t, addItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.RemoveItem(ctx, cartID, productID))

		// Same predicate CountItems used before it was dropped as dead code: no
		// production caller ever read the count it returned.
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM cart_items WHERE cart_id = $1`, cartID).Scan(&count))
		assert.Equal(t, 0, count)
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

	t.Run("GetOrCreate", func(t *testing.T) {
		_, err := repo.GetOrCreate(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("RemoveItem", func(t *testing.T) {
		err := repo.RemoveItem(cancelledCtx, uuid.New(), uuid.New())
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

// addItem seeds a line directly: this package owns no AddItem repository
// method of its own, only the removal this test exercises.
func addItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
	_, err := testPool.Exec(ctx,
		`INSERT INTO cart_items (cart_id, product_id, quantity) VALUES ($1, $2, $3)`,
		cartID, productID, qty,
	)
	return err
}

// cleanupCart deletes the cart row; cart_items cascades from it. Registered
// after seedUser so it runs first in t.Cleanup's LIFO order -- the user's own
// row can only be deleted once its cart is gone.
func cleanupCart(t *testing.T, cartID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM carts WHERE id = $1`, cartID) })
}
