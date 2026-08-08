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
	pool, cleanup := testhelper.MustStartPostgres("test_dashboard")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetSalesSummary(t *testing.T) {
	t.Run("returns zero stats when no paid orders in range", func(t *testing.T) {
		repo := New(testPool)

		from := time.Now().Add(100 * 24 * time.Hour)
		to := time.Now().Add(200 * 24 * time.Hour)

		summary, err := repo.GetSalesSummary(context.Background(), from, to)
		require.NoError(t, err)
		assert.Equal(t, 0, summary.TotalOrders)
		assert.Equal(t, int64(0), summary.TotalRevenue)
		assert.InDelta(t, float64(0), summary.AverageOrderValue, 0.001)
	})

	t.Run("returns correct stats for paid orders", func(t *testing.T) {
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := New(testPool)

		from := time.Now().Add(-24 * time.Hour)
		to := time.Now().Add(24 * time.Hour)

		summary, err := repo.GetSalesSummary(context.Background(), from, to)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, summary.TotalOrders, 1)
		assert.GreaterOrEqual(t, summary.TotalRevenue, int64(1000))
	})
}

func TestPostgresRepository_GetOrderStatusBreakdown(t *testing.T) {
	t.Run("returns breakdown including seeded order status", func(t *testing.T) {
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := New(testPool)

		breakdowns, err := repo.GetOrderStatusBreakdown(context.Background(),
			time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
		require.NoError(t, err)
		assert.NotEmpty(t, breakdowns)

		var found bool
		for _, b := range breakdowns {
			if b.Status == "paid" {
				found = true
				assert.GreaterOrEqual(t, b.Count, 1)
				break
			}
		}
		assert.True(t, found, "expected 'paid' status to appear in breakdown")
	})
}

func TestPostgresRepository_GetSalesSummary_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetSalesSummary(ctx, time.Now(), time.Now())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetOrderStatusBreakdown_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetOrderStatusBreakdown(ctx, time.Now().Add(-24*time.Hour), time.Now())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedPaidOrder(t *testing.T, userID uuid.UUID) {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'paid', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
}
