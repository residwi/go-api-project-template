package wishlist_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/wishlist"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_wishlist")
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
	t.Run("creates wishlist on first call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)

		wishlistID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, wishlistID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)

		first, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)

		second, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("adds product to wishlist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))

		has, err := repo.HasItem(ctx, wishlistID, productID)
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("silently ignores duplicate (ON CONFLICT DO NOTHING)", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		// Second insert must not error
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
	})
}

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		require.NoError(t, repo.RemoveItem(ctx, wishlistID, productID))

		has, err := repo.HasItem(ctx, wishlistID, productID)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := wishlist.NewPostgresRepository(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetItems(t *testing.T) {
	t.Run("returns empty list when wishlist does not exist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)

		items, err := repo.GetItems(context.Background(), userID, core.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("returns items with pagination cursor when results exceed limit", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)

		// Add 5 products
		for range 5 {
			productID := seedProduct(t)
			require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		}

		// Fetch with limit 3 — returns limit+1 to detect more
		items, err := repo.GetItems(ctx, userID, core.CursorPage{Limit: 3})
		require.NoError(t, err)
		assert.Len(t, items, 4) // limit+1 signals more available
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		for range 5 {
			productID := seedProduct(t)
			require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		}

		page1, err := repo.GetItems(ctx, userID, core.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.GetItems(ctx, userID, core.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, item := range page2 {
			assert.NotEqual(t, page1[0].ID, item.ID)
			assert.NotEqual(t, page1[1].ID, item.ID)
		}
	})
}

func TestPostgresRepository_HasItem(t *testing.T) {
	t.Run("returns false when item not in wishlist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		has, err := repo.HasItem(ctx, wishlistID, uuid.New())
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns true when item is in wishlist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))

		has, err := repo.HasItem(ctx, wishlistID, productID)
		require.NoError(t, err)
		assert.True(t, has)
	})
}

func TestPostgresRepository_GetItems_InvalidCursor(t *testing.T) {
	t.Run("returns error for invalid cursor", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := wishlist.NewPostgresRepository(testPool)
		ctx := context.Background()

		_, _ = repo.GetOrCreate(ctx, userID)

		_, err := repo.GetItems(ctx, userID, core.CursorPage{Cursor: "!!!invalid!!!", Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := wishlist.NewPostgresRepository(testPool)

	t.Run("GetOrCreate", func(t *testing.T) {
		setup(t)
		_, err := repo.GetOrCreate(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("AddItem", func(t *testing.T) {
		setup(t)
		err := repo.AddItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("RemoveItem", func(t *testing.T) {
		setup(t)
		err := repo.RemoveItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetItems", func(t *testing.T) {
		setup(t)
		_, err := repo.GetItems(cancelledCtx, uuid.New(), core.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("HasItem", func(t *testing.T) {
		setup(t)
		_, err := repo.HasItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}
