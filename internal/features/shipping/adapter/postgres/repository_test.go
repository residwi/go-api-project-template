package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_shipping")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates shipment with correct fields", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

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
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

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

		repo := New(database.DB{Primary: testPool})
		s := &domain.Shipment{
			OrderID: uuid.New(),
			Carrier: "FedEx",
			Status:  domain.StatusPending,
		}
		err := repo.Create(cancelledCtx, s)
		assert.Error(t, err)
	})

	t.Run("rolls back the insert when the enclosing transaction fails", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		tx := database.NewTxRunner(testPool)

		s := &domain.Shipment{
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "ROLLBACK123",
			Status:         domain.StatusPending,
		}

		wantErr := errors.New("order flip failed")
		err := tx.Run(context.Background(), func(ctx context.Context) error {
			if err := repo.Create(ctx, s); err != nil {
				return err
			}
			return wantErr
		})
		require.ErrorIs(t, err, wantErr)
		assert.NotEqual(t, uuid.Nil, s.ID, "Create must still populate the id from RETURNING before rollback")

		var count int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM shipments WHERE id = $1`, s.ID).Scan(&count))
		assert.Equal(t, 0, count, "shipment must not survive a transaction that rolled back")
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns shipment", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "UPS", domain.StatusShipped)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), shipmentID)
		require.NoError(t, err)
		assert.Equal(t, shipmentID, got.ID)
		assert.Equal(t, orderID, got.OrderID)
		assert.Equal(t, "UPS", got.Carrier)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(database.DB{Primary: testPool})
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetByOrderID(t *testing.T) {
	t.Run("returns shipment for order", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "DHL", domain.StatusPending)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, shipmentID, got.ID)
		assert.Equal(t, orderID, got.OrderID)
		assert.Equal(t, "DHL", got.Carrier)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByOrderID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(database.DB{Primary: testPool})
		_, err := repo.GetByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_MarkDelivered(t *testing.T) {
	t.Run("sets delivered_at and status to delivered", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "UPS", domain.StatusShipped)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.MarkDelivered(context.Background(), shipmentID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusDelivered, got.Status)
		assert.NotNil(t, got.DeliveredAt)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.MarkDelivered(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(database.DB{Primary: testPool})
		_, err := repo.MarkDelivered(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates carrier and tracking number", func(t *testing.T) {
		userID := testutil.SeedUser(t, testPool)
		orderID := seedOrder(t, userID)
		shipmentID := seedShipment(t, orderID, "OldCarrier", domain.StatusPending)
		repo := New(database.DB{Primary: testPool})
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
		repo := New(database.DB{Primary: testPool})

		s := &domain.Shipment{
			ID:      uuid.New(),
			Carrier: "Ghost",
			Status:  domain.StatusPending,
		}
		err := repo.Update(context.Background(), s)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(database.DB{Primary: testPool})
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

// seedShipment inserts directly rather than through Repository.Create: some
// subtests exercise GetByID, GetByOrderID, MarkDelivered or Update against a
// row Create's own defaults (shipped_at, timestamps) would not let them
// control.
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
