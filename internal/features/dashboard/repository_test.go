package dashboard_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/dashboard"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_dashboard")
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

func seedPaidOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'paid', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
	return id
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity)
		 VALUES ($1, 'Widget', $2, 'desc', 1000, 'USD', 10)`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

func seedOrderItem(t *testing.T, orderID, productID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO order_items (id, order_id, product_id, product_name, price, quantity, subtotal)
		 VALUES ($1, $2, $3, 'Widget', 1000, 2, 2000)`,
		id, orderID, productID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM order_items WHERE id = $1`, id) })
	return id
}

func TestPostgresRepository_GetSalesSummary(t *testing.T) {
	t.Run("returns zero stats when no paid orders in range", func(t *testing.T) {
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)

		// Use a time range far in the future with no data
		from := time.Now().Add(100 * 24 * time.Hour)
		to := time.Now().Add(200 * 24 * time.Hour)

		summary, err := repo.GetSalesSummary(context.Background(), from, to)
		require.NoError(t, err)
		assert.Equal(t, 0, summary.TotalOrders)
		assert.Equal(t, int64(0), summary.TotalRevenue)
		assert.InDelta(t, float64(0), summary.AverageOrderValue, 0.001)
	})

	t.Run("returns correct stats for paid orders", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := dashboard.NewPostgresRepository(testPool)

		from := time.Now().Add(-24 * time.Hour)
		to := time.Now().Add(24 * time.Hour)

		summary, err := repo.GetSalesSummary(context.Background(), from, to)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, summary.TotalOrders, 1)
		assert.GreaterOrEqual(t, summary.TotalRevenue, int64(1000))
	})
}

func TestPostgresRepository_GetTopProducts(t *testing.T) {
	t.Run("returns empty slice when no orders", func(t *testing.T) {
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)

		// Use a time range far in the future with no data
		from := time.Now().Add(100 * 24 * time.Hour)
		to := time.Now().Add(200 * 24 * time.Hour)

		products, err := repo.GetTopProducts(context.Background(), 10, from, to)
		require.NoError(t, err)
		assert.Empty(t, products)
	})

	t.Run("returns top products from paid orders", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedPaidOrder(t, userID)
		productID := seedProduct(t)
		seedOrderItem(t, orderID, productID)
		repo := dashboard.NewPostgresRepository(testPool)

		from := time.Now().Add(-24 * time.Hour)
		to := time.Now().Add(24 * time.Hour)

		products, err := repo.GetTopProducts(context.Background(), 10, from, to)
		require.NoError(t, err)
		assert.NotEmpty(t, products)

		var found bool
		for _, p := range products {
			if p.ProductID == productID {
				found = true
				assert.Equal(t, "Widget", p.Name)
				assert.GreaterOrEqual(t, p.TotalSold, 2)
				break
			}
		}
		assert.True(t, found, "expected seeded product to appear in top products")
	})
}

func TestPostgresRepository_GetRevenueByDay(t *testing.T) {
	t.Run("returns revenue grouped by day", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := dashboard.NewPostgresRepository(testPool)

		from := time.Now().Add(-24 * time.Hour)
		to := time.Now().Add(24 * time.Hour)

		data, err := repo.GetRevenueByDay(context.Background(), from, to)
		require.NoError(t, err)
		assert.NotEmpty(t, data)

		for _, d := range data {
			assert.False(t, d.Date.IsZero())
			assert.GreaterOrEqual(t, d.Revenue, int64(0))
			assert.GreaterOrEqual(t, d.OrderCount, 1)
		}
	})
}

func TestPostgresRepository_GetOrderStatusBreakdown(t *testing.T) {
	t.Run("returns breakdown including seeded order status", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := dashboard.NewPostgresRepository(testPool)

		breakdowns, err := repo.GetOrderStatusBreakdown(context.Background())
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
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetSalesSummary(ctx, time.Now(), time.Now())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetTopProducts_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetTopProducts(ctx, 10, time.Now(), time.Now())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetRevenueByDay_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetRevenueByDay(ctx, time.Now(), time.Now())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetOrderStatusBreakdown_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := dashboard.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetOrderStatusBreakdown(ctx)
		assert.Error(t, err)
	})
}
