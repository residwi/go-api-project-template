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
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_inventory")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Reserve(t *testing.T) {
	t.Run("reserves available stock", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)

		stock, err := repo.Reserve(context.Background(), productID, 3)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 10, stock.Quantity)
		assert.Equal(t, 3, stock.Reserved)
		assert.Equal(t, 7, stock.Available)
	})

	t.Run("returns insufficient stock error when not enough", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)

		_, err := repo.Reserve(context.Background(), productID, 100)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})
}

func TestPostgresRepository_ReleaseBatch(t *testing.T) {
	t.Run("releases the reservation for every product", func(t *testing.T) {
		setup(t)
		first := seedProduct(t)
		second := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.ReserveBatch(ctx, []inventory.StockChange{
			{ProductID: first, Quantity: 4},
			{ProductID: second, Quantity: 2},
		}))

		require.NoError(t, repo.ReleaseBatch(ctx, []inventory.StockChange{
			{ProductID: first, Quantity: 4},
			{ProductID: second, Quantity: 2},
		}))

		assert.Equal(t, 0, reservedOf(t, first))
		assert.Equal(t, 0, reservedOf(t, second))
	})

	// Silent success here would strand the reservation forever.
	t.Run("refuses to release more than is reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.ReserveBatch(ctx, []inventory.StockChange{
			{ProductID: productID, Quantity: 2},
		}))

		err := repo.ReleaseBatch(ctx, []inventory.StockChange{
			{ProductID: productID, Quantity: 3},
		})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.Equal(t, 2, reservedOf(t, productID), "the reservation must be left intact")
	})

	t.Run("no items is a no-op", func(t *testing.T) {
		setup(t)
		repo := New(testPool)

		assert.NoError(t, repo.ReleaseBatch(context.Background(), nil))
	})
}

func TestPostgresRepository_ReserveBatch_DuplicateProduct(t *testing.T) {
	t.Run("sums quantities for a repeated product id instead of failing", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		// The same product appears twice; quantities must be summed (2+3=5),
		// not joined to the product row twice (which previously made
		// RowsAffected < len(items) and wrongly reported insufficient stock).
		err := repo.ReserveBatch(ctx, []inventory.StockChange{
			{ProductID: productID, Quantity: 2},
			{ProductID: productID, Quantity: 3},
		})
		require.NoError(t, err)

		stock, err := repo.GetStock(ctx, productID)
		require.NoError(t, err)
		assert.Equal(t, &inventory.Stock{ProductID: productID, Quantity: 10, Reserved: 5, Available: 5}, stock)
	})
}

func TestPostgresRepository_Release(t *testing.T) {
	t.Run("releases reserved stock", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 5)
		require.NoError(t, err)

		stock, err := repo.Release(ctx, productID, 3)
		require.NoError(t, err)
		assert.Equal(t, 2, stock.Reserved)
		assert.Equal(t, 8, stock.Available)
	})

	t.Run("returns error when releasing more than reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 2)
		require.NoError(t, err)

		_, err = repo.Release(ctx, productID, 5)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}

func TestPostgresRepository_Deduct(t *testing.T) {
	t.Run("deducts stock and reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 4)
		require.NoError(t, err)

		stock, err := repo.Deduct(ctx, productID, 4)
		require.NoError(t, err)
		assert.Equal(t, 6, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 6, stock.Available)
	})

	t.Run("returns error when not enough reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 2)
		require.NoError(t, err)

		_, err = repo.Deduct(ctx, productID, 5)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}

func TestPostgresRepository_Restock(t *testing.T) {
	t.Run("adds to stock quantity", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)

		stock, err := repo.Restock(context.Background(), productID, 5)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 15, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 15, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		setup(t)
		repo := New(testPool)

		_, err := repo.Restock(context.Background(), uuid.New(), 5)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetStock(t *testing.T) {
	t.Run("returns stock for product", func(t *testing.T) {
		setup(t)
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
		setup(t)
		repo := New(testPool)

		_, err := repo.GetStock(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_AdjustStock(t *testing.T) {
	t.Run("adjusts to new quantity", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)

		stock, err := repo.AdjustStock(context.Background(), productID, 20)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 20, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 20, stock.Available)
	})

	t.Run("returns error when new quantity below reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 5)
		require.NoError(t, err)

		_, err = repo.AdjustStock(ctx, productID, 3)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	// GetStock and Restock both 404 on a missing level row, so AdjustStock's upsert
	// is the only way to recover a product whose EnsureLevel never ran.
	t.Run("succeeds against a product with no level row", func(t *testing.T) {
		setup(t)
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

		repo := New(testPool)
		stock, err := repo.AdjustStock(ctx, id, 15)
		require.NoError(t, err)
		assert.Equal(t, &inventory.Stock{ProductID: id, Quantity: 15, Reserved: 0, Available: 15}, stock)
	})
}

func TestPostgresRepository_EnsureLevel(t *testing.T) {
	t.Run("creates a zeroed level row for a product with none", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		repo := New(testPool)

		id := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, description, price, currency)
			 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
			id, "slug-"+id.String())
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })

		require.NoError(t, repo.EnsureLevel(ctx, id))

		stock, err := repo.GetStock(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, &inventory.Stock{ProductID: id, Quantity: 0, Reserved: 0, Available: 0}, stock)
	})

	t.Run("is idempotent and does not clobber an existing reservation", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		repo := New(testPool)

		productID := seedProduct(t)
		_, err := repo.Reserve(ctx, productID, 3)
		require.NoError(t, err)

		require.NoError(t, repo.EnsureLevel(ctx, productID))

		stock, err := repo.GetStock(ctx, productID)
		require.NoError(t, err)
		assert.Equal(t, 3, stock.Reserved, "a retry must not reset an existing reservation")
	})
}

func TestPostgresRepository_GetLevels(t *testing.T) {
	t.Run("returns levels for many products in one call", func(t *testing.T) {
		setup(t)
		repo := New(testPool)

		id1 := seedProduct(t)
		id2 := seedProduct(t)
		_, err := repo.Reserve(context.Background(), id1, 4)
		require.NoError(t, err)

		levels, err := repo.GetLevels(context.Background(), []uuid.UUID{id1, id2})
		require.NoError(t, err)
		require.Len(t, levels, 2)
		assert.Equal(t, inventory.Stock{ProductID: id1, Quantity: 10, Reserved: 4, Available: 6}, levels[id1])
		assert.Equal(t, inventory.Stock{ProductID: id2, Quantity: 10, Reserved: 0, Available: 10}, levels[id2])
	})

	t.Run("omits ids with no level row", func(t *testing.T) {
		setup(t)
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
		setup(t)
		repo := New(testPool)

		levels, err := repo.GetLevels(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, levels)
	})
}

func TestPostgresRepository_Reserve_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Reserve(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Release_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Release(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Deduct_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Deduct(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Restock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Restock(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStock(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_AdjustStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.AdjustStock(ctx, uuid.New(), 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_EnsureLevel_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.EnsureLevel(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetLevels_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetLevels(ctx, []uuid.UUID{uuid.New()})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ReserveBatch_UsesInventoryLevels(t *testing.T) {
	setup(t)
	ctx := context.Background()
	repo := New(testPool)

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 10, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 10, reserved_stock = 0`, productID)
	require.NoError(t, err)

	require.NoError(t, repo.ReserveBatch(ctx, []inventory.StockChange{
		{ProductID: productID, Quantity: 4},
	}))

	var available, reserved int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	assert.Equal(t, 6, available)
	assert.Equal(t, 4, reserved)
}

func TestPostgresRepository_ReserveBatch_RejectsOverReservation(t *testing.T) {
	setup(t)
	ctx := context.Background()
	repo := New(testPool)

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 2, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 2, reserved_stock = 0`, productID)
	require.NoError(t, err)

	err = repo.ReserveBatch(ctx, []inventory.StockChange{{ProductID: productID, Quantity: 5}})
	require.ErrorIs(t, err, apperror.ErrInsufficientStock)

	var available int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available))
	assert.Equal(t, 2, available, "a rejected batch must reserve nothing")
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

// seedLevel upserts an inventory_levels row directly, bypassing the repository
// under test so tests can arrange a starting available/reserved split.
func seedLevel(t *testing.T, productID uuid.UUID, available, reserved int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = EXCLUDED.available_stock,
		     reserved_stock  = EXCLUDED.reserved_stock`,
		productID, available, reserved)
	require.NoError(t, err)
}

// Both rows: inventory_levels.product_id references products, and the repository
// under test reads and writes inventory_levels exclusively.
func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	seedLevel(t, id, 10, 0)
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
