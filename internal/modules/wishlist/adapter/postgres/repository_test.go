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
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_wishlist")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetOrCreate(t *testing.T) {
	t.Run("creates wishlist on first call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		wishlistID, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, wishlistID)
	})

	t.Run("returns same id on second call", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		first, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)

		second, err := repo.GetOrCreate(context.Background(), userID)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}

func TestPostgresRepository_AddItem(t *testing.T) {
	t.Run("adds product to wishlist", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		wishlistID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))

		assert.True(t, itemExists(t, wishlistID, productID))
	})

	t.Run("silently ignores duplicate (ON CONFLICT DO NOTHING)", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		wishlistID, err := repo.GetOrCreate(ctx, userID)
		require.NoError(t, err)
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
		require.NoError(t, repo.AddItem(ctx, wishlistID, productID))
	})
}

func TestPostgresRepository_RemoveItem(t *testing.T) {
	t.Run("removes existing item", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		wishlistID := seedWishlist(t, userID)
		seedItem(t, wishlistID, productID)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.RemoveItem(ctx, userID, productID))

		assert.False(t, itemExists(t, wishlistID, productID))
	})

	t.Run("returns not found when item does not exist", func(t *testing.T) {
		repo := New(testPool)
		err := repo.RemoveItem(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_ListItemsForUser(t *testing.T) {
	t.Run("returns empty list when wishlist does not exist", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		items, err := repo.ListItemsForUser(context.Background(), userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("returns items with pagination cursor when results exceed limit", func(t *testing.T) {
		userID := seedUser(t)
		wishlistID := seedWishlist(t, userID)
		repo := New(testPool)

		for range 5 {
			seedItem(t, wishlistID, seedProduct(t))
		}

		// Fetch with limit 3 — returns limit+1 to detect more
		items, err := repo.ListItemsForUser(context.Background(), userID, paging.CursorPage{Limit: 3})
		require.NoError(t, err)
		assert.Len(t, items, 4) // limit+1 signals more available
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		userID := seedUser(t)
		wishlistID := seedWishlist(t, userID)
		repo := New(testPool)

		for range 5 {
			seedItem(t, wishlistID, seedProduct(t))
		}

		page1, err := repo.ListItemsForUser(context.Background(), userID, paging.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListItemsForUser(context.Background(), userID, paging.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, item := range page2 {
			assert.NotEqual(t, page1[0].ID, item.ID)
			assert.NotEqual(t, page1[1].ID, item.ID)
		}
	})

	t.Run("returns error for invalid cursor", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		_, err := repo.ListItemsForUser(
			context.Background(), userID, paging.CursorPage{Cursor: "!!!invalid!!!", Limit: 10},
		)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetOrCreate", func(t *testing.T) {
		_, err := repo.GetOrCreate(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("AddItem", func(t *testing.T) {
		err := repo.AddItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("RemoveItem", func(t *testing.T) {
		err := repo.RemoveItem(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListItemsForUser", func(t *testing.T) {
		_, err := repo.ListItemsForUser(cancelledCtx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testutil.SeedUser(t, testPool)
}

func seedWishlist(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO wishlists (id, user_id) VALUES ($1, $2)`, id, userID)
	require.NoError(t, err)
	return id
}

func seedItem(t *testing.T, wishlistID, productID uuid.UUID) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO wishlist_items (wishlist_id, product_id) VALUES ($1, $2)`, wishlistID, productID)
	require.NoError(t, err)
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

func itemExists(t *testing.T, wishlistID, productID uuid.UUID) bool {
	t.Helper()
	var exists bool
	err := testPool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM wishlist_items WHERE wishlist_id = $1 AND product_id = $2)`,
		wishlistID, productID,
	).Scan(&exists)
	require.NoError(t, err)
	return exists
}
