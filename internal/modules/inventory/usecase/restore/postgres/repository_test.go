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

func TestPostgresRepository_ReleaseBatch(t *testing.T) {
	t.Run("releases the reservation for every product", func(t *testing.T) {
		first := seedProduct(t)
		second := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		arrangeReservation(t, first, 4)
		arrangeReservation(t, second, 2)

		require.NoError(t, repo.ReleaseBatch(ctx, map[uuid.UUID]int{
			first:  4,
			second: 2,
		}))

		assert.Equal(t, 0, reservedOf(t, first))
		assert.Equal(t, 0, reservedOf(t, second))
	})

	// Silent success here would strand the reservation forever.
	t.Run("refuses to release more than is reserved", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		arrangeReservation(t, productID, 2)

		err := repo.ReleaseBatch(ctx, map[uuid.UUID]int{productID: 3})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.Equal(t, 2, reservedOf(t, productID), "the reservation must be left intact")
	})

	t.Run("no items is a no-op", func(t *testing.T) {
		repo := New(testPool)

		assert.NoError(t, repo.ReleaseBatch(context.Background(), nil))
	})
}

func TestPostgresRepository_Release(t *testing.T) {
	t.Run("releases reserved stock", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		arrangeReservation(t, productID, 5)

		stock, err := repo.Release(ctx, productID, 3)
		require.NoError(t, err)
		assert.Equal(t, 2, stock.Reserved)
		assert.Equal(t, 8, stock.Available)
	})

	t.Run("returns error when releasing more than reserved", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		arrangeReservation(t, productID, 2)

		_, err := repo.Release(ctx, productID, 5)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}

func TestPostgresRepository_Release_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Release(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

// arrangeReservation moves stock into the reserved column directly -- the
// same predicate reserve/postgres.Reserve runs -- bypassing the restore
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

func reservedOf(t *testing.T, productID uuid.UUID) int {
	t.Helper()
	var reserved int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT reserved_stock FROM inventory_levels WHERE product_id = $1`, productID).Scan(&reserved))
	return reserved
}
