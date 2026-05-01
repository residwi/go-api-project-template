package shipping_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_shipping")
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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates shipment with correct fields", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)

		s := &shipping.Shipment{
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusPending,
		}
		err := repo.Create(context.Background(), s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM shipments WHERE id = $1`, s.ID) })

		assert.NotEqual(t, uuid.Nil, s.ID)
		assert.Equal(t, orderID, s.OrderID)
		assert.Equal(t, "FedEx", s.Carrier)
		assert.Equal(t, "TRACK123", s.TrackingNumber)
		assert.Equal(t, shipping.StatusPending, s.Status)
		assert.False(t, s.CreatedAt.IsZero())
		assert.False(t, s.UpdatedAt.IsZero())
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns shipment", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)
		ctx := context.Background()

		s := &shipping.Shipment{
			OrderID: orderID,
			Carrier: "UPS",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(ctx, s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM shipments WHERE id = $1`, s.ID) })

		got, err := repo.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, s.ID, got.ID)
		assert.Equal(t, s.OrderID, got.OrderID)
		assert.Equal(t, "UPS", got.Carrier)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := shipping.NewPostgresRepository(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetByOrderID(t *testing.T) {
	t.Run("returns shipment for order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)
		ctx := context.Background()

		s := &shipping.Shipment{
			OrderID: orderID,
			Carrier: "DHL",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(ctx, s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM shipments WHERE id = $1`, s.ID) })

		got, err := repo.GetByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Equal(t, s.ID, got.ID)
		assert.Equal(t, orderID, got.OrderID)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := shipping.NewPostgresRepository(testPool)

		_, err := repo.GetByOrderID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates carrier and tracking number", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)
		ctx := context.Background()

		s := &shipping.Shipment{
			OrderID: orderID,
			Carrier: "OldCarrier",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(ctx, s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM shipments WHERE id = $1`, s.ID) })

		s.Carrier = "NewCarrier"
		s.TrackingNumber = "NEW-TRACK-456"
		err = repo.Update(ctx, s)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, "NewCarrier", got.Carrier)
		assert.Equal(t, "NEW-TRACK-456", got.TrackingNumber)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := shipping.NewPostgresRepository(testPool)

		s := &shipping.Shipment{
			ID:      uuid.New(),
			Carrier: "Ghost",
			Status:  shipping.StatusPending,
		}
		err := repo.Update(context.Background(), s)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_MarkShipped(t *testing.T) {
	t.Run("sets shipped_at and status to shipped", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)
		ctx := context.Background()

		s := &shipping.Shipment{
			OrderID: orderID,
			Carrier: "FedEx",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(ctx, s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM shipments WHERE id = $1`, s.ID) })

		err = repo.MarkShipped(ctx, s.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, shipping.StatusShipped, got.Status)
		assert.NotNil(t, got.ShippedAt)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := shipping.NewPostgresRepository(testPool)

		err := repo.MarkShipped(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_MarkDelivered(t *testing.T) {
	t.Run("sets delivered_at and status to delivered", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := shipping.NewPostgresRepository(testPool)
		ctx := context.Background()

		s := &shipping.Shipment{
			OrderID: orderID,
			Carrier: "UPS",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(ctx, s)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM shipments WHERE id = $1`, s.ID) })

		err = repo.MarkDelivered(ctx, s.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, shipping.StatusDelivered, got.Status)
		assert.NotNil(t, got.DeliveredAt)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := shipping.NewPostgresRepository(testPool)

		err := repo.MarkDelivered(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := shipping.NewPostgresRepository(testPool)

	t.Run("Create", func(t *testing.T) {
		setup(t)
		s := &shipping.Shipment{
			OrderID: uuid.New(),
			Carrier: "FedEx",
			Status:  shipping.StatusPending,
		}
		err := repo.Create(cancelledCtx, s)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetByOrderID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		setup(t)
		s := &shipping.Shipment{ID: uuid.New(), Carrier: "UPS", Status: shipping.StatusPending}
		err := repo.Update(cancelledCtx, s)
		assert.Error(t, err)
	})

	t.Run("MarkShipped", func(t *testing.T) {
		setup(t)
		err := repo.MarkShipped(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkDelivered", func(t *testing.T) {
		setup(t)
		err := repo.MarkDelivered(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}
