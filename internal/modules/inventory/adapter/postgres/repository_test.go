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
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// This package owns test_inventory outright now that every inventory slice
// merged into one adapter -- one TestMain, one MustStartPostgres call. It
// still never truncates: every row it touches is seeded here with a fresh
// uuid.New() and cleaned up by name, the same discipline the shared database
// needed before the merge.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_inventory")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_AdjustStock(t *testing.T) {
	t.Run("adjusts to new quantity", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		stock, err := repo.AdjustStock(context.Background(), productID, 20)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 20, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 20, stock.Available)
	})

	t.Run("returns error when new quantity below reserved", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		arrangeReservation(t, productID, 5)

		_, err := repo.AdjustStock(ctx, productID, 3)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	// GetStock and Restock both 404 on a missing level row, so AdjustStock's
	// upsert is the only way to recover a product whose EnsureLevel never ran.
	t.Run("succeeds against a product with no level row", func(t *testing.T) {
		ctx := context.Background()
		id := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, description, price, currency)
			 VALUES ($1, 'Levelless', $2, 'desc', 1000, 'USD')`,
			id, "levelless-"+id.String())
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM inventory_levels WHERE product_id = $1`, id)
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
		})

		repo := New(database.DB{Primary: testPool})
		stock, err := repo.AdjustStock(ctx, id, 15)
		require.NoError(t, err)
		assert.Equal(t, &domain.Stock{ProductID: id, Quantity: 15, Reserved: 0, Available: 15}, stock)
	})
}

func TestPostgresRepository_AdjustStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.AdjustStock(ctx, uuid.New(), 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Restock(t *testing.T) {
	t.Run("adds to stock quantity", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		stock, err := repo.Restock(context.Background(), productID, 5)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 15, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 15, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.Restock(context.Background(), uuid.New(), 5)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Restock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Restock(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_EnsureLevel(t *testing.T) {
	t.Run("creates a zeroed level row for a product with none", func(t *testing.T) {
		ctx := context.Background()
		repo := New(database.DB{Primary: testPool})

		id := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, description, price, currency)
			 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
			id, "slug-"+id.String())
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM inventory_levels WHERE product_id = $1`, id)
			testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
		})

		require.NoError(t, repo.EnsureLevel(ctx, id))

		available, reserved := levelOf(t, id)
		assert.Equal(t, 0, available)
		assert.Equal(t, 0, reserved)
	})

	t.Run("is idempotent and does not clobber an existing reservation", func(t *testing.T) {
		ctx := context.Background()
		repo := New(database.DB{Primary: testPool})

		productID := seedProduct(t)
		arrangeReservation(t, productID, 3)

		require.NoError(t, repo.EnsureLevel(ctx, productID))

		_, reserved := levelOf(t, productID)
		assert.Equal(t, 3, reserved, "a retry must not reset an existing reservation")
	})
}

func TestPostgresRepository_EnsureLevel_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.EnsureLevel(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStock(t *testing.T) {
	t.Run("returns stock for product", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(database.DB{Primary: testPool})

		stock, err := repo.GetStock(context.Background(), productID)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 10, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 10, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetStock(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStock(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetLevels(t *testing.T) {
	t.Run("returns levels for many products in one call", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

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
		repo := New(database.DB{Primary: testPool})

		id := seedProduct(t)
		missing := uuid.New()

		levels, err := repo.GetLevels(context.Background(), []uuid.UUID{id, missing})
		require.NoError(t, err)
		assert.Len(t, levels, 1)
		_, ok := levels[missing]
		assert.False(t, ok)
	})

	t.Run("empty ids returns empty map without querying", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		levels, err := repo.GetLevels(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, levels)
	})
}

func TestPostgresRepository_GetLevels_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetLevels(ctx, []uuid.UUID{uuid.New()})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Reserve_UsesInventoryLevels(t *testing.T) {
	ctx := context.Background()
	repo := New(database.DB{Primary: testPool})

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 10, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 10, reserved_stock = 0`, productID)
	require.NoError(t, err)

	require.NoError(t, repo.Reserve(ctx, map[uuid.UUID]int{productID: 4}))

	var available, reserved int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	assert.Equal(t, 6, available)
	assert.Equal(t, 4, reserved)
}

func TestPostgresRepository_Reserve_RejectsOverReservation(t *testing.T) {
	ctx := context.Background()
	repo := New(database.DB{Primary: testPool})

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 2, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 2, reserved_stock = 0`, productID)
	require.NoError(t, err)

	err = repo.Reserve(ctx, map[uuid.UUID]int{productID: 5})
	require.ErrorIs(t, err, apperror.ErrInsufficientStock)

	var available int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available))
	assert.Equal(t, 2, available, "a rejected batch must reserve nothing")
}

// Deduct had no postgres-level test before this flatten -- deduct's own
// repository_test.go covered only the now-deleted singular Deduct. Added
// here, mirroring Reserve's pair, now that Deduct is the sole surviving
// deduction path.
func TestPostgresRepository_Deduct_UsesInventoryLevels(t *testing.T) {
	ctx := context.Background()
	repo := New(database.DB{Primary: testPool})

	productID := seedProduct(t)
	arrangeReservation(t, productID, 4)

	require.NoError(t, repo.Deduct(ctx, map[uuid.UUID]int{productID: 4}))

	var available, reserved int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	assert.Equal(t, 6, available)
	assert.Equal(t, 0, reserved)
}

func TestPostgresRepository_Deduct_RejectsOverDeduction(t *testing.T) {
	ctx := context.Background()
	repo := New(database.DB{Primary: testPool})

	productID := seedProduct(t)
	arrangeReservation(t, productID, 2)

	err := repo.Deduct(ctx, map[uuid.UUID]int{productID: 5})
	require.ErrorIs(t, err, apperror.ErrBadRequest)

	_, reserved := levelOf(t, productID)
	assert.Equal(t, 2, reserved, "a rejected batch must leave the reservation intact")
}

func TestPostgresRepository_ReleaseBatch(t *testing.T) {
	t.Run("releases the reservation for every product", func(t *testing.T) {
		first := seedProduct(t)
		second := seedProduct(t)
		repo := New(database.DB{Primary: testPool})
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
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		arrangeReservation(t, productID, 2)

		err := repo.ReleaseBatch(ctx, map[uuid.UUID]int{productID: 3})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.Equal(t, 2, reservedOf(t, productID), "the reservation must be left intact")
	})

	t.Run("no items is a no-op", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		assert.NoError(t, repo.ReleaseBatch(context.Background(), nil))
	})
}

// arrangeReservation moves stock into the reserved column directly -- the
// same predicate Reserve runs -- bypassing the repository under test so a
// subtest can arrange a starting reservation.
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
// reserved) with a fresh id, and cleans up both on test completion -- every
// row is still scoped by id rather than truncated, the discipline the seven
// slices sharing this database used before they merged into one package.
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

func levelOf(t *testing.T, productID uuid.UUID) (available, reserved int) {
	t.Helper()
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	return available, reserved
}

func reservedOf(t *testing.T, productID uuid.UUID) int {
	t.Helper()
	var reserved int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT reserved_stock FROM inventory_levels WHERE product_id = $1`, productID).Scan(&reserved))
	return reserved
}
