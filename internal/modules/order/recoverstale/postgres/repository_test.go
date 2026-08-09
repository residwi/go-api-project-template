package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetStaleProcessingOrders(t *testing.T) {
	t.Run("returns orders stuck in payment_processing beyond threshold", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
		ctx := context.Background()

		staleID := uuid.New()
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, updated_at)
			 VALUES ($1, $2, 'payment_processing', 1000, 0, 1000, 'USD', NOW() - INTERVAL '1 hour')`,
			staleID,
			userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, staleID) })

		orders, err := repo.GetStaleProcessingOrders(ctx, 30*time.Minute, 10)
		require.NoError(t, err)

		var found bool
		for _, o := range orders {
			if o.ID == staleID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected stale processing order to be returned")
	})
}

func TestPostgresRepository_GetStaleProcessingOrders_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStaleProcessingOrders(ctx, 30*time.Minute, 10)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}
