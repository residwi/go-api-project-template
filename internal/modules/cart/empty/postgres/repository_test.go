package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_cart")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Clear(t *testing.T) {
	t.Run("removes all items from cart", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID := seedCart(t, userID)
		require.NoError(t, addItem(ctx, cartID, productID, 3))
		require.NoError(t, repo.Clear(ctx, userID))

		// Same predicate CountItems used before it was dropped as dead code: no
		// production caller ever read the count it returned.
		var count int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM cart_items WHERE cart_id = $1`, cartID).Scan(&count))
		require.Equal(t, 0, count)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("Clear", func(t *testing.T) {
		err := repo.Clear(cancelledCtx, uuid.New())
		require.Error(t, err)
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

// seedCart inserts a cart directly: this package's Repository owns only
// Clear, not the create-on-first-use path add/updatequantity/remove test.
func seedCart(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var cartID uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID).Scan(&cartID)
	require.NoError(t, err)
	// cart_items cascades from carts, so one delete clears both.
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM carts WHERE id = $1`, cartID) })
	return cartID
}

func addItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error {
	_, err := testPool.Exec(ctx,
		`INSERT INTO cart_items (cart_id, product_id, quantity) VALUES ($1, $2, $3)`,
		cartID, productID, qty,
	)
	return err
}
