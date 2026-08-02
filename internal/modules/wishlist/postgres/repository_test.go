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
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
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
	t.Run("creates wishlist on first call", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)

		wishlistID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, wishlistID)
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

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("adds product to wishlist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := postgres.New(testPool)
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
		repo := postgres.New(testPool)
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
		repo := postgres.New(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		require.NoError(t, repo.RemoveItem(ctx, userID, productID))

		has, err := repo.HasItem(ctx, wishlistID, productID)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		setup(t)
		repo := postgres.New(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetItems(t *testing.T) {
	t.Run("returns empty list when wishlist does not exist", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)

		items, err := repo.GetItems(context.Background(), userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("returns items with pagination cursor when results exceed limit", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)

		// Add 5 products
		for range 5 {
			productID := seedProduct(t)
			require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		}

		// Fetch with limit 3 — returns limit+1 to detect more
		items, err := repo.GetItems(ctx, userID, paging.CursorPage{Limit: 3})
		require.NoError(t, err)
		assert.Len(t, items, 4) // limit+1 signals more available
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := postgres.New(testPool)
		ctx := context.Background()

		wishlistID, _ := repo.GetOrCreate(ctx, userID)
		for range 5 {
			productID := seedProduct(t)
			require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		}

		page1, err := repo.GetItems(ctx, userID, paging.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.GetItems(ctx, userID, paging.CursorPage{Cursor: cursor, Limit: 2})
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
		repo := postgres.New(testPool)
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
		repo := postgres.New(testPool)
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
		repo := postgres.New(testPool)
		ctx := context.Background()

		_, _ = repo.GetOrCreate(ctx, userID)

		_, err := repo.GetItems(ctx, userID, paging.CursorPage{Cursor: "!!!invalid!!!", Limit: 10})
		assert.Error(t, err)
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
		_, err := repo.GetItems(cancelledCtx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("HasItem", func(t *testing.T) {
		setup(t)
		_, err := repo.HasItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})
}
