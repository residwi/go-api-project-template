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

func TestPostgresRepository_Reserve(t *testing.T) {
	t.Run("reserves available stock", func(t *testing.T) {
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
		productID := seedProduct(t)
		repo := New(testPool)

		_, err := repo.Reserve(context.Background(), productID, 100)
		assert.ErrorIs(t, err, apperror.ErrInsufficientStock)
	})
}

func TestPostgresRepository_Reserve_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Reserve(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ReserveBatch_UsesInventoryLevels(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool)

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 10, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 10, reserved_stock = 0`, productID)
	require.NoError(t, err)

	require.NoError(t, repo.ReserveBatch(ctx, map[uuid.UUID]int{productID: 4}))

	var available, reserved int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	assert.Equal(t, 6, available)
	assert.Equal(t, 4, reserved)
}

func TestPostgresRepository_ReserveBatch_RejectsOverReservation(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool)

	productID := seedProduct(t)
	_, err := testPool.Exec(ctx,
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, 2, 0)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = 2, reserved_stock = 0`, productID)
	require.NoError(t, err)

	err = repo.ReserveBatch(ctx, map[uuid.UUID]int{productID: 5})
	require.ErrorIs(t, err, apperror.ErrInsufficientStock)

	var available int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT available_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available))
	assert.Equal(t, 2, available, "a rejected batch must reserve nothing")
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
