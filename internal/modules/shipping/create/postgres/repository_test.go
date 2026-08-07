package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_shipping")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates shipment with correct fields", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		repo := New(testPool)

		s := &domain.Shipment{
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         domain.StatusPending,
		}
		err := repo.Create(context.Background(), s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM shipments WHERE id = $1`, s.ID) })

		assert.NotEqual(t, uuid.Nil, s.ID)
		assert.Equal(t, orderID, s.OrderID)
		assert.Equal(t, "FedEx", s.Carrier)
		assert.Equal(t, "TRACK123", s.TrackingNumber)
		assert.Equal(t, domain.StatusPending, s.Status)
		assert.False(t, s.CreatedAt.IsZero())
		assert.False(t, s.UpdatedAt.IsZero())
	})

	t.Run("sets shipped_at when status is shipped", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		repo := New(testPool)

		s := &domain.Shipment{
			OrderID:        orderID,
			Carrier:        "UPS",
			TrackingNumber: "UPS123",
			Status:         domain.StatusShipped,
		}
		err := repo.Create(context.Background(), s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM shipments WHERE id = $1`, s.ID) })

		require.NotNil(t, s.ShippedAt)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		s := &domain.Shipment{
			OrderID: uuid.New(),
			Carrier: "FedEx",
			Status:  domain.StatusPending,
		}
		err := repo.Create(cancelledCtx, s)
		assert.Error(t, err)
	})
}

func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
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
