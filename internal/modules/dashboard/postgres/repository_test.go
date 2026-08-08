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
	pool, cleanup := testhelper.MustStartPostgres("test_features_dashboard")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetRevenueByDay(t *testing.T) {
	t.Run("returns revenue grouped by day", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedPaidOrder(t, userID)
		repo := New(testPool)

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

func TestPostgresRepository_GetRevenueByDay_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetRevenueByDay(ctx, time.Now(), time.Now())
		assert.Error(t, err)
	})
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
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
