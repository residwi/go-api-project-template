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
	"github.com/residwi/go-api-project-template/internal/modules/review/domain"
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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates review", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := New(testPool)

		rv := &domain.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    5,
			Title:     "Great product",
			Body:      "Really loved it.",
			Status:    "published",
		}
		err := repo.Create(context.Background(), rv)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM reviews WHERE id = $1`, rv.ID) })

		assert.NotEqual(t, uuid.Nil, rv.ID)
		assert.Equal(t, userID, rv.UserID)
		assert.Equal(t, productID, rv.ProductID)
		assert.Equal(t, orderID, rv.OrderID)
		assert.Equal(t, 5, rv.Rating)
		assert.Equal(t, "published", rv.Status)
		assert.False(t, rv.CreatedAt.IsZero())
	})

	t.Run("returns conflict on duplicate user+product", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := New(testPool)
		ctx := context.Background()

		first := &domain.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    4,
			Status:    "published",
		}
		err := repo.Create(ctx, first)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, first.ID) })

		second := &domain.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    3,
			Status:    "published",
		}
		err = repo.Create(ctx, second)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_HasUserReviewed(t *testing.T) {
	t.Run("returns false when user has not reviewed", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := New(testPool)

		has, err := repo.HasUserReviewed(context.Background(), userID, productID)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns true when user has reviewed", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := New(testPool)
		ctx := context.Background()

		rv := &domain.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    4,
			Status:    "published",
		}
		err := repo.Create(ctx, rv)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, rv.ID) })

		has, err := repo.HasUserReviewed(ctx, userID, productID)
		require.NoError(t, err)
		assert.True(t, has)
	})
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

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes review", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		id := seedReview(t, userID, productID, orderID, "published")
		repo := New(testPool)
		ctx := context.Background()

		err := repo.Delete(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, 0, reviewCount(t, id), "the row must not survive Delete")
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("Create", func(t *testing.T) {
		rv := &domain.Review{
			UserID:    uuid.New(),
			ProductID: uuid.New(),
			OrderID:   uuid.New(),
			Rating:    5,
			Status:    "published",
		}
		err := repo.Create(cancelledCtx, rv)
		assert.Error(t, err)
	})

	t.Run("HasUserReviewed", func(t *testing.T) {
		_, err := repo.HasUserReviewed(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListByProduct", func(t *testing.T) {
		_, err := repo.ListByProduct(cancelledCtx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("GetStats", func(t *testing.T) {
		_, err := repo.GetStats(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		err := repo.Delete(cancelledCtx, uuid.New())
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

func reviewCount(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM reviews WHERE id = $1`, id).Scan(&count))
	return count
}
