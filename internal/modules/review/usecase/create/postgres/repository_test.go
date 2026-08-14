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
