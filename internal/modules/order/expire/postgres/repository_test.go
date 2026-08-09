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
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetExpiredOrders(t *testing.T) {
	t.Run("returns awaiting_payment orders older than 30 minutes", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
		ctx := context.Background()

		oldOrderID := uuid.New()
		// GetExpiredOrders sorts oldest-first with a LIMIT, and test_order is a
		// shared database that is never truncated (see the registry comment in
		// internal/testhelper/testhelper.go), so accumulated rows from earlier
		// runs sort ahead of this one. 100 years is arbitrary -- it only needs to
		// predate every row that has ever accumulated so this order is always
		// oldest and never crowded out of the LIMIT.
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, created_at)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD', NOW() - INTERVAL '100 years')`,
			oldOrderID,
			userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, oldOrderID) })

		orders, err := repo.GetExpiredOrders(ctx, 10)
		require.NoError(t, err)

		var found bool
		for _, o := range orders {
			if o.ID == oldOrderID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected old awaiting_payment order to appear in expired orders")
	})

	t.Run("does not return recent orders", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID) // created now -- not expired
		repo := New(testPool)

		orders, err := repo.GetExpiredOrders(context.Background(), 100)
		require.NoError(t, err)

		for _, got := range orders {
			assert.NotEqual(t, orderID, got.ID)
		}
	})
}

func TestPostgresRepository_GetExpiredOrders_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetExpiredOrders(ctx, 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListItemsByOrderID(t *testing.T) {
	t.Run("returns all items for order", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Widget', 500, 1, 500)`, orderID, productID)
		require.NoError(t, err)

		repo := New(testPool)
		got, err := repo.ListItemsByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})
}

func TestPostgresRepository_ListItemsByOrderID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListItemsByOrderID(ctx, uuid.New())
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
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

// seedOrder inserts a minimal order row: raw SQL, not place's repository, so
// this package never imports a sibling slice.
func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var orderID uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO orders (user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD') RETURNING id`,
		userID, uuid.New().String(),
	).Scan(&orderID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}
