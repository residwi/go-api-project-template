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
	pool, cleanup := testhelper.MustStartPostgres("test_cart")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetCart(t *testing.T) {
	t.Run("returns empty cart when no cart exists for user", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		c, err := repo.GetCart(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, userID, c.UserID)
		assert.Empty(t, c.Items)
	})

	t.Run("returns cart with items", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID := seedCart(t, userID)
		require.NoError(t, addItem(ctx, cartID, productID, 2))

		c, err := repo.GetCart(ctx, userID)
		require.NoError(t, err)
		require.Len(t, c.Items, 1)
		assert.Equal(t, cartID, c.Items[0].CartID)
		assert.Equal(t, productID, c.Items[0].ProductID)
		assert.Equal(t, 2, c.Items[0].Quantity)
		assert.Nil(
			t,
			c.Items[0].Product,
			"repository no longer joins products; the reader fills this in through ProductLookup",
		)
	})

	t.Run("keeps the line when its product is soft-deleted", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID := seedCart(t, userID)
		require.NoError(t, addItem(ctx, cartID, productID, 1))

		_, err := testPool.Exec(ctx, `UPDATE products SET deleted_at = NOW() WHERE id = $1`, productID)
		require.NoError(t, err)

		c, err := repo.GetCart(ctx, userID)
		require.NoError(t, err)
		require.Len(t, c.Items, 1, "a soft-deleted product's line must not silently vanish from the cart")
		assert.Equal(t, productID, c.Items[0].ProductID)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetCart", func(t *testing.T) {
		_, err := repo.GetCart(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

// The six cross-module cascades were unreachable (users and products are
// soft-deleted); the four within-module ones are aggregate-internal and
// correct. Needs no seeded row: it reads the constraint catalog directly.
func TestCrossModuleCascadesDropped(t *testing.T) {
	noAction := []string{
		"carts_user_id_fkey",
		"cart_items_product_id_fkey",
		"wishlists_user_id_fkey",
		"wishlist_items_product_id_fkey",
		"notifications_user_id_fkey",
		"notification_jobs_user_id_fkey",
	}
	stillCascade := []string{
		"product_images_product_id_fkey",
		"cart_items_cart_id_fkey",
		"order_items_order_id_fkey",
		"wishlist_items_wishlist_id_fkey",
	}

	for _, name := range noAction {
		t.Run(name+" does not cascade", func(t *testing.T) {
			var delType string
			require.NoError(t, testPool.QueryRow(context.Background(),
				`SELECT confdeltype::text FROM pg_constraint WHERE conname = $1`, name).Scan(&delType))
			assert.Equal(t, "a", delType, "expected NO ACTION")
		})
	}
	for _, name := range stillCascade {
		t.Run(name+" still cascades", func(t *testing.T) {
			var delType string
			require.NoError(t, testPool.QueryRow(context.Background(),
				`SELECT confdeltype::text FROM pg_constraint WHERE conname = $1`, name).Scan(&delType))
			assert.Equal(t, "c", delType, "aggregate-internal cascade must be kept")
		})
	}
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
// GetCart, not the create-on-first-use path add/updatequantity/remove test.
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
