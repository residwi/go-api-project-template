package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
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

func TestPostgresRepository_GetOrCreate(t *testing.T) {
	t.Run("creates cart on first call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)

		cartID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, cartID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)

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
		repo := postgres.New(testPool)

		c, err := repo.GetCart(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, userID, c.UserID)
		assert.Empty(t, c.Items)
	})

	t.Run("returns cart with items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 2))

		c, err := repo.GetCart(ctx, userID)
		require.NoError(t, err)
		require.Len(t, c.Items, 1)
		assert.Equal(t, cartID, c.Items[0].CartID)
		assert.Equal(t, productID, c.Items[0].ProductID)
		assert.Equal(t, 2, c.Items[0].Quantity)
		assert.Nil(t, c.Items[0].Product, "repository no longer joins products; the service fills this in through ProductLookup")
	})

	t.Run("keeps the line when its product is soft-deleted", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))

		_, err := testPool.Exec(ctx, `UPDATE products SET deleted_at = NOW() WHERE id = $1`, productID)
		require.NoError(t, err)

		c, err := repo.GetCart(ctx, userID)
		require.NoError(t, err)
		require.Len(t, c.Items, 1, "a soft-deleted product's line must not silently vanish from the cart")
		assert.Equal(t, productID, c.Items[0].ProductID)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("accumulates quantity on duplicate insert", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
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
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.UpdateItemQuantity(ctx, cartID, productID, 5))

		c, _ := repo.GetCart(ctx, userID)
		assert.Equal(t, 5, c.Items[0].Quantity)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		err := repo.UpdateItemQuantity(context.Background(), uuid.New(), uuid.New(), 5)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.RemoveItem(ctx, cartID, productID))

		c, _ := repo.GetCart(ctx, userID)
		assert.Empty(t, c.Items)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Clear(t *testing.T) {
	t.Run("removes all items from cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 3))
		require.NoError(t, repo.Clear(ctx, userID))

		count, _ := repo.CountItems(ctx, cartID)
		assert.Equal(t, 0, count)
	})
}

func TestPostgresRepository_CountItems(t *testing.T) {
	t.Run("returns zero for empty cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)
		cartID, _ := repo.GetOrCreate(context.Background(), userID)

		count, err := repo.CountItems(context.Background(), cartID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns correct count after adding items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))

		count, err := repo.CountItems(ctx, cartID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestPostgresRepository_CountAndHasItem(t *testing.T) {
	t.Run("returns zero count and false when the product is not in the cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)

		count, hasProduct, err := repo.CountAndHasItem(ctx, cartID, productID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.False(t, hasProduct)
	})

	t.Run("returns the distinct count and true when the product is in the cart", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		otherID := seedProduct(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		cartID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, cartID, productID, 1))
		require.NoError(t, repo.AddItem(ctx, cartID, otherID, 1))

		count, hasProduct, err := repo.CountAndHasItem(ctx, cartID, productID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.True(t, hasProduct)
	})
}

func TestPostgresRepository_GetCartForLock(t *testing.T) {
	t.Run("returns not found when cart does not exist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)

		_, err := repo.GetCartForLock(context.Background(), userID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns cart id when cart exists", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)
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

	repo := postgres.New(testPool)

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

	t.Run("CountAndHasItem", func(t *testing.T) {
		setup(t)
		_, _, err := repo.CountAndHasItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetCartForLock", func(t *testing.T) {
		setup(t)
		_, err := repo.GetCartForLock(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

// TestUserHardDeleteIsRestricted documents that carts no longer disappear when a
// user row is hard-deleted. The application only ever soft-deletes users
// (UPDATE users SET deleted_at), so the old ON DELETE CASCADE was unreachable
// configuration that implied cart cleanup the code never performed.
func TestUserHardDeleteIsRestricted(t *testing.T) {
	setup(t)
	ctx := context.Background()

	userID := seedUser(t)
	repo := postgres.New(testPool)
	_, err := repo.GetOrCreate(ctx, userID)
	require.NoError(t, err)

	_, err = testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.Error(t, err, "hard-deleting a user with a cart must be refused")
	assert.True(t, database.IsForeignKeyViolation(err),
		"expected a foreign key violation, got: %v", err)

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM carts WHERE user_id = $1`, userID).Scan(&count))
	assert.Equal(t, 1, count, "cart must survive the refused delete")
}

// TestCrossModuleCascadesDropped pins which foreign keys cascade. The six
// cross-module ones were unreachable (users and products are soft-deleted) while
// implying a cleanup the code never performed; the four within-module ones are
// aggregate-internal and correct.
func TestCrossModuleCascadesDropped(t *testing.T) {
	setup(t)

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
