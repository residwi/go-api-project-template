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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// This package shares test_inventory with every other inventory slice's
// postgres/ package. It never resets or truncates -- every row it touches is
// seeded here with a fresh uuid.New() and cleaned up by name.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_inventory")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetStock(t *testing.T) {
	t.Run("returns stock for product", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)

		stock, err := repo.GetStock(context.Background(), productID)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 10, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 10, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetStock(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStock(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetLevels(t *testing.T) {
	t.Run("returns levels for many products in one call", func(t *testing.T) {
		repo := New(testPool)

		id1 := seedProduct(t)
		id2 := seedProduct(t)
		arrangeReservation(t, id1, 4)

		levels, err := repo.GetLevels(context.Background(), []uuid.UUID{id1, id2})
		require.NoError(t, err)
		require.Len(t, levels, 2)
		assert.Equal(t, domain.Stock{ProductID: id1, Quantity: 10, Reserved: 4, Available: 6}, levels[id1])
		assert.Equal(t, domain.Stock{ProductID: id2, Quantity: 10, Reserved: 0, Available: 10}, levels[id2])
	})

	t.Run("omits ids with no level row", func(t *testing.T) {
		repo := New(testPool)

		id := seedProduct(t)
		missing := uuid.New()

		levels, err := repo.GetLevels(context.Background(), []uuid.UUID{id, missing})
		require.NoError(t, err)
		assert.Len(t, levels, 1)
		_, ok := levels[missing]
		assert.False(t, ok)
	})

	t.Run("empty ids returns empty map without querying", func(t *testing.T) {
		repo := New(testPool)

		levels, err := repo.GetLevels(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, levels)
	})
}

func TestPostgresRepository_GetLevels_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetLevels(ctx, []uuid.UUID{uuid.New()})
		assert.Error(t, err)
	})
}

// arrangeReservation moves stock into the reserved column directly -- the
// same predicate reserve/postgres.Reserve runs -- bypassing the query
// package under test so a subtest can arrange a starting reservation.
func arrangeReservation(t *testing.T, productID uuid.UUID, qty int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`UPDATE inventory_levels
		 SET available_stock = available_stock - $1, reserved_stock = reserved_stock + $1
		 WHERE product_id = $2 AND available_stock >= $1`,
		qty, productID)
	require.NoError(t, err)
}

// seedProduct inserts a product and its inventory_levels row (10 available, 0
// reserved) with a fresh id, and cleans up both on test completion -- this
// package never truncates a table it shares with every other inventory slice.
func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 10, 0)`, id)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM inventory_levels WHERE product_id = $1`, id)
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
	return id
}
