package cart_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_cart")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity)
		 VALUES ($1, 'Test Product', $2, 'desc', 1000, 'USD', 10)`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

func TestPostgresRepository_GetOrCreate(t *testing.T) {
	t.Run("creates cart on first call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)

		cartID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, cartID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)

		first, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)

		second, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}

func TestPostgresRepository_GetCart(t *testing.T) {
	t.Run("returns empty cart when no cart exists for user", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)

		c, err := repo.GetCart(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, userID, c.UserID)
		assert.Empty(t, c.Items)
	})

	t.Run("returns cart with items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 2))

		c, err := repo.GetCart(ctx, userID)
		require.NoError(t, err)
		require.Len(t, c.Items, 1)
		assert.Equal(t, cartID, c.Items[0].CartID)
		assert.Equal(t, productID, c.Items[0].ProductID)
		assert.Equal(t, 2, c.Items[0].Quantity)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("accumulates quantity on duplicate insert", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 2))
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 3))

		count, _ := repo.CountItems(ctx, cartID)
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_UpdateItemQuantity(t *testing.T) {
	t.Run("updates quantity of existing item", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.UpdateItemQuantity(ctx, cartID, productID, 5))

		c, _ := repo.GetCart(ctx, userID)
		assert.Equal(t, 5, c.Items[0].Quantity)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := cart.NewPostgresRepository(testPool)
		err := repo.UpdateItemQuantity(context.Background(), uuid.New(), uuid.New(), 5)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.RemoveItem(ctx, cartID, productID))

		c, _ := repo.GetCart(ctx, userID)
		assert.Empty(t, c.Items)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := cart.NewPostgresRepository(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_Clear(t *testing.T) {
	t.Run("removes all items from cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 3))
		require.NoError(t, repo.Clear(ctx, cartID))

		count, _ := repo.CountItems(ctx, cartID)
		assert.Equal(t, 0, count)
	})
}

func TestPostgresRepository_CountItems(t *testing.T) {
	t.Run("returns zero for empty cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)
		cartID, _ := repo.GetOrCreate(context.Background(), userID)

		count, err := repo.CountItems(context.Background(), cartID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns correct count after adding items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))

		count, err := repo.CountItems(ctx, cartID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_GetCartForLock(t *testing.T) {
	t.Run("returns not found when cart does not exist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)

		_, err := repo.GetCartForLock(context.Background(), userID)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("returns cart id when cart exists", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := cart.NewPostgresRepository(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		lockedID, err := repo.GetCartForLock(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, cartID, lockedID)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := cart.NewPostgresRepository(testPool)

	t.Run("GetOrCreate", func(t *testing.T) {
		setup(t)
		_, err := repo.GetOrCreate(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetCart", func(t *testing.T) {
		setup(t)
		_, err := repo.GetCart(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("AddItem", func(t *testing.T) {
		setup(t)
		err := repo.AddItem(cancelledCtx, uuid.New(), uuid.New(), 1)
		assert.Error(t, err)
	})

	t.Run("UpdateItemQuantity", func(t *testing.T) {
		setup(t)
		err := repo.UpdateItemQuantity(cancelledCtx, uuid.New(), uuid.New(), 1)
		assert.Error(t, err)
	})

	t.Run("RemoveItem", func(t *testing.T) {
		setup(t)
		err := repo.RemoveItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("Clear", func(t *testing.T) {
		setup(t)
		err := repo.Clear(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("CountItems", func(t *testing.T) {
		setup(t)
		_, err := repo.CountItems(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetCartForLock", func(t *testing.T) {
		setup(t)
		_, err := repo.GetCartForLock(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}
