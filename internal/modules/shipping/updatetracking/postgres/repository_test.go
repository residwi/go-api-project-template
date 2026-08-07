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

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns shipment", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "UPS", domain.StatusPending)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), shipmentID)
		require.NoError(t, err)
		assert.Equal(t, shipmentID, got.ID)
		assert.Equal(t, orderID, got.OrderID)
		assert.Equal(t, "UPS", got.Carrier)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates carrier and tracking number", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "OldCarrier", domain.StatusPending)
		repo := New(testPool)
		ctx := context.Background()

		s := &domain.Shipment{
			ID:             shipmentID,
			Carrier:        "NewCarrier",
			TrackingNumber: "NEW-TRACK-456",
			Status:         domain.StatusPending,
		}
		err := repo.Update(ctx, s)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, shipmentID)
		require.NoError(t, err)
		assert.Equal(t, "NewCarrier", got.Carrier)
		assert.Equal(t, "NEW-TRACK-456", got.TrackingNumber)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		s := &domain.Shipment{
			ID:      uuid.New(),
			Carrier: "Ghost",
			Status:  domain.StatusPending,
		}
		err := repo.Update(context.Background(), s)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		s := &domain.Shipment{ID: uuid.New(), Carrier: "UPS", Status: domain.StatusPending}
		err := repo.Update(cancelledCtx, s)
		assert.Error(t, err)
	})
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

// seedShipment inserts directly rather than through Repository: this package's
// Repository only has GetByID and Update, since updatetracking never creates a
// shipment.
func seedShipment(t *testing.T, orderID uuid.UUID, carrier string, status domain.ShipmentStatus) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		// tracking_number is nullable in the schema but a plain string on
		// domain.Shipment, so it must be inserted as "" rather than left NULL.
		`INSERT INTO shipments (id, order_id, carrier, tracking_number, status) VALUES ($1, $2, $3, '', $4)`,
		id, orderID, carrier, status,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM shipments WHERE id = $1`, id) })
	return id
}
