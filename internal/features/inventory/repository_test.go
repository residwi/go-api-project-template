package inventory_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_inventory")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD', 10)`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
	return id
}

func TestPostgresRepository_Reserve(t *testing.T) {
	t.Run("reserves available stock", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)

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
		repo := inventory.NewPostgresRepository(testPool)

		_, err := repo.Reserve(context.Background(), productID, 100)
		assert.ErrorIs(t, err, core.ErrInsufficientStock)
	})
}

func TestPostgresRepository_Release(t *testing.T) {
	t.Run("releases reserved stock", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)
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
		repo := inventory.NewPostgresRepository(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 2)
		require.NoError(t, err)

		_, err = repo.Release(ctx, productID, 5)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})
}

func TestPostgresRepository_Deduct(t *testing.T) {
	t.Run("deducts stock and reserved", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)
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
		repo := inventory.NewPostgresRepository(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 2)
		require.NoError(t, err)

		_, err = repo.Deduct(ctx, productID, 5)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})
}

func TestPostgresRepository_Restock(t *testing.T) {
	t.Run("adds to stock quantity", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)

		stock, err := repo.Restock(context.Background(), productID, 5)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 15, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 15, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)

		_, err := repo.Restock(context.Background(), uuid.New(), 5)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetStock(t *testing.T) {
	t.Run("returns stock for product", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)

		stock, err := repo.GetStock(context.Background(), productID)
		require.NoError(t, err)
		assert.Equal(t, productID, stock.ProductID)
		assert.Equal(t, 10, stock.Quantity)
		assert.Equal(t, 0, stock.Reserved)
		assert.Equal(t, 10, stock.Available)
	})

	t.Run("returns not found for unknown product", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)

		_, err := repo.GetStock(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_AdjustStock(t *testing.T) {
	t.Run("adjusts to new quantity", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := inventory.NewPostgresRepository(testPool)

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
		repo := inventory.NewPostgresRepository(testPool)
		ctx := context.Background()

		_, err := repo.Reserve(ctx, productID, 5)
		require.NoError(t, err)

		_, err = repo.AdjustStock(ctx, productID, 3)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})
}

func TestPostgresRepository_Reserve_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Reserve(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Release_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Release(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Deduct_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Deduct(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Restock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.Restock(ctx, uuid.New(), 1)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStock(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_AdjustStock_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := inventory.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.AdjustStock(ctx, uuid.New(), 10)
		assert.Error(t, err)
	})
}
