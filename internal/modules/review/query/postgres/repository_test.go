package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_review")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_ListByProduct(t *testing.T) {
	t.Run("returns only published reviews", func(t *testing.T) {
		userA := seedUser(t)
		userB := seedUser(t)
		productID := seedProduct(t)
		orderA := seedOrder(t, userA)
		orderB := seedOrder(t, userB)
		repo := New(testPool)
		ctx := context.Background()

		published := seedReview(t, userA, productID, orderA, "published")
		seedReview(t, userB, productID, orderB, "pending")

		reviews, err := repo.ListByProduct(ctx, productID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		require.Len(t, reviews, 1)
		assert.Equal(t, published, reviews[0].ID)
		assert.Equal(t, "published", reviews[0].Status)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)
		ctx := context.Background()

		for range 4 {
			userID := seedUser(t)
			orderID := seedOrder(t, userID)
			seedReview(t, userID, productID, orderID, "published")
		}

		page1, err := repo.ListByProduct(ctx, productID, paging.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByProduct(ctx, productID, paging.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
	})
}

func TestPostgresRepository_ListByProduct_InvalidCursor(t *testing.T) {
	t.Run("returns error for invalid cursor", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)

		_, err := repo.ListByProduct(
			context.Background(),
			productID,
			paging.CursorPage{Cursor: "!!!invalid!!!", Limit: 10},
		)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStats(t *testing.T) {
	t.Run("returns zero stats when no reviews", func(t *testing.T) {
		productID := seedProduct(t)
		repo := New(testPool)

		stats, err := repo.GetStats(context.Background(), productID)
		require.NoError(t, err)
		assert.InDelta(t, float64(0), stats.AverageRating, 0.001)
		assert.Equal(t, 0, stats.TotalReviews)
	})

	t.Run("returns stats for published reviews", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := New(testPool)

		seedReview(t, userID, productID, orderID, "published")

		stats, err := repo.GetStats(context.Background(), productID)
		require.NoError(t, err)
		assert.InDelta(t, float64(5), stats.AverageRating, 0.001)
		assert.Equal(t, 1, stats.TotalReviews)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("ListByProduct", func(t *testing.T) {
		_, err := repo.ListByProduct(cancelledCtx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("GetStats", func(t *testing.T) {
		_, err := repo.GetStats(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
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

func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
	return id
}

// seedReview inserts a review with the given status directly, bypassing
// create's Repository -- query's own tests have no reason to depend on a
// sibling slice's adapter.
func seedReview(t *testing.T, userID, productID, orderID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO reviews (id, user_id, product_id, order_id, rating, title, body, status)
		 VALUES ($1, $2, $3, $4, 5, 'Review', 'Body', $5)`,
		id, userID, productID, orderID, status,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM reviews WHERE id = $1`, id) })
	return id
}
