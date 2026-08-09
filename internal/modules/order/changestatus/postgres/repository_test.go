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
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns order", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, orderID, got.ID)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetByID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetByID(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

// seedOrder inserts a minimal order row: raw SQL, not place's repository, so
// this package never imports a sibling slice.
func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var orderID uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO orders (user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'paid', 1000, 0, 1000, 'USD') RETURNING id`,
		userID, uuid.New().String(),
	).Scan(&orderID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}
