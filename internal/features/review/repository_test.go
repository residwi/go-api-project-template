package review_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/review"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_review")
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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates review", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)

		rv := &review.Review{
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
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		first := &review.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    4,
			Status:    "published",
		}
		err := repo.Create(ctx, first)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, first.ID) })

		second := &review.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    3,
			Status:    "published",
		}
		err = repo.Create(ctx, second)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns review", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		rv := &review.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    5,
			Status:    "published",
		}
		err := repo.Create(ctx, rv)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, rv.ID) })

		got, err := repo.GetByID(ctx, rv.ID)
		require.NoError(t, err)
		assert.Equal(t, rv.ID, got.ID)
		assert.Equal(t, rv.Rating, got.Rating)
		assert.Equal(t, rv.Status, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := review.NewPostgresRepository(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_ListByProduct(t *testing.T) {
	t.Run("returns only published reviews", func(t *testing.T) {
		setup(t)
		userA := seedUser(t)
		userB := seedUser(t)
		productID := seedProduct(t)
		orderA := seedOrder(t, userA)
		orderB := seedOrder(t, userB)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		published := &review.Review{
			UserID:    userA,
			ProductID: productID,
			OrderID:   orderA,
			Rating:    5,
			Title:     "Published",
			Status:    "published",
		}
		err := repo.Create(ctx, published)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, published.ID) })

		pending := &review.Review{
			UserID:    userB,
			ProductID: productID,
			OrderID:   orderB,
			Rating:    3,
			Title:     "Pending",
			Status:    "pending",
		}
		err = repo.Create(ctx, pending)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, pending.ID) })

		reviews, err := repo.ListByProduct(ctx, productID, core.CursorPage{Limit: 10})
		require.NoError(t, err)
		require.Len(t, reviews, 1)
		assert.Equal(t, published.ID, reviews[0].ID)
		assert.Equal(t, "published", reviews[0].Status)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		for range 4 {
			userID := seedUser(t)
			orderID := seedOrder(t, userID)
			r := &review.Review{
				UserID: userID, ProductID: productID, OrderID: orderID,
				Rating: 4, Title: "Review", Status: "published",
			}
			require.NoError(t, repo.Create(ctx, r))
			t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, r.ID) })
		}

		page1, err := repo.ListByProduct(ctx, productID, core.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(page1), 2)

		last := page1[1]
		cursor := core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByProduct(ctx, productID, core.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
	})
}

func TestPostgresRepository_GetStats(t *testing.T) {
	t.Run("returns zero stats when no reviews", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := review.NewPostgresRepository(testPool)

		stats, err := repo.GetStats(context.Background(), productID)
		require.NoError(t, err)
		assert.InDelta(t, float64(0), stats.AverageRating, 0.001)
		assert.Equal(t, 0, stats.TotalReviews)
	})

	t.Run("returns stats for published reviews", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		rv := &review.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    5,
			Status:    "published",
		}
		err := repo.Create(ctx, rv)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, rv.ID) })

		stats, err := repo.GetStats(ctx, productID)
		require.NoError(t, err)
		assert.InDelta(t, float64(5), stats.AverageRating, 0.001)
		assert.Equal(t, 1, stats.TotalReviews)
	})
}

func TestPostgresRepository_HasUserReviewed(t *testing.T) {
	t.Run("returns false when user has not reviewed", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		repo := review.NewPostgresRepository(testPool)

		has, err := repo.HasUserReviewed(context.Background(), userID, productID)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns true when user has reviewed", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		rv := &review.Review{
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

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes review", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		repo := review.NewPostgresRepository(testPool)
		ctx := context.Background()

		rv := &review.Review{
			UserID:    userID,
			ProductID: productID,
			OrderID:   orderID,
			Rating:    3,
			Status:    "published",
		}
		err := repo.Create(ctx, rv)
		require.NoError(t, err)

		err = repo.Delete(ctx, rv.ID)
		require.NoError(t, err)

		_, err = repo.GetByID(ctx, rv.ID)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := review.NewPostgresRepository(testPool)

		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_ListByProduct_InvalidCursor(t *testing.T) {
	t.Run("returns error for invalid cursor", func(t *testing.T) {
		setup(t)
		productID := seedProduct(t)
		repo := review.NewPostgresRepository(testPool)

		_, err := repo.ListByProduct(context.Background(), productID, core.CursorPage{Cursor: "!!!invalid!!!", Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := review.NewPostgresRepository(testPool)

	t.Run("Create", func(t *testing.T) {
		setup(t)
		rv := &review.Review{
			UserID:    uuid.New(),
			ProductID: uuid.New(),
			OrderID:   uuid.New(),
			Rating:    5,
			Status:    "published",
		}
		err := repo.Create(cancelledCtx, rv)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListByProduct", func(t *testing.T) {
		setup(t)
		_, err := repo.ListByProduct(cancelledCtx, uuid.New(), core.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("GetStats", func(t *testing.T) {
		setup(t)
		_, err := repo.GetStats(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("HasUserReviewed", func(t *testing.T) {
		setup(t)
		_, err := repo.HasUserReviewed(cancelledCtx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		setup(t)
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}
