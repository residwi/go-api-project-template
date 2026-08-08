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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_review")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes review", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID)
		id := seedReview(t, userID, productID, orderID)
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

func seedReview(t *testing.T, userID, productID, orderID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO reviews (id, user_id, product_id, order_id, rating, title, status)
		 VALUES ($1, $2, $3, $4, 3, 'Review', 'published')`,
		id, userID, productID, orderID,
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
