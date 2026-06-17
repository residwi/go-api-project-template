package shipping_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	mocks "github.com/residwi/go-api-project-template/mocks/shipping"
)

// noopDBTX satisfies database.DBTX so WithTestTx can seed a tx, letting
// CreateShipment's WithTx run as a passthrough with a nil pool.
type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func TestService_CreateShipment(t *testing.T) {
	t.Run("success order in paid status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		userID := uuid.New()

		orders.EXPECT().GetByID(mock.Anything, orderID).
			Return(shipping.OrderInfo{
				ID:     orderID,
				UserID: userID,
				Status: "paid",
			}, nil)

		shippedID := uuid.New()
		now := time.Now()
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).
			Run(func(_ context.Context, s *shipping.Shipment) {
				s.ID = shippedID
				s.ShippedAt = &now
				s.CreatedAt = now
				s.UpdatedAt = now
			}).
			Return(nil)

		updater.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"paid", "processing"}, "shipped").
			Return(nil)

		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateShipmentRequest{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
		})
		require.NoError(t, err)
		assert.Equal(t, shippedID, result.ID)
		assert.Equal(t, orderID, result.OrderID)
		assert.Equal(t, "FedEx", result.Carrier)
		assert.Equal(t, "TRACK123", result.TrackingNumber)
		assert.Equal(t, shipping.StatusShipped, result.Status)
		assert.Equal(t, &now, result.ShippedAt)
	})

	t.Run("order wrong status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).
			Return(shipping.OrderInfo{
				ID:     orderID,
				Status: "pending",
			}, nil)

		_, err := svc.CreateShipment(context.Background(), orderID, shipping.CreateShipmentRequest{
			Carrier:        "UPS",
			TrackingNumber: "UPS123",
		})
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("order not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).
			Return(shipping.OrderInfo{}, core.ErrNotFound)

		_, err := svc.CreateShipment(context.Background(), orderID, shipping.CreateShipmentRequest{
			Carrier:        "DHL",
			TrackingNumber: "DHL456",
		})
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("repo create error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID:     orderID,
			UserID: uuid.New(),
			Status: "paid",
		}, nil)

		dbErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).Return(dbErr)

		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateShipmentRequest{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
		})
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("update order status error rolls back", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID:     orderID,
			UserID: uuid.New(),
			Status: "paid",
		}, nil)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).
			Run(func(_ context.Context, s *shipping.Shipment) {
				s.ID = uuid.New()
			}).Return(nil)

		updateErr := errors.New("order status update failed")
		updater.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"paid", "processing"}, "shipped").Return(updateErr)

		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateShipmentRequest{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
		})
		assert.Nil(t, result)
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_GetByOrderID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		expected := &shipping.Shipment{
			ID:             uuid.New(),
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK999",
			Status:         shipping.StatusShipped,
		}
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(expected, nil)

		result, err := svc.GetByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		orderID := uuid.New()
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		result, err := svc.GetByOrderID(context.Background(), orderID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_UpdateTracking(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		existing := &shipping.Shipment{
			ID:             shipmentID,
			OrderID:        uuid.New(),
			Carrier:        "FedEx",
			TrackingNumber: "OLD123",
			Status:         shipping.StatusShipped,
		}

		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).Return(nil)

		updated := &shipping.Shipment{
			ID:             shipmentID,
			OrderID:        existing.OrderID,
			Carrier:        "UPS",
			TrackingNumber: "NEW456",
			Status:         shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(updated, nil).Once()

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingRequest{
			Carrier:        "UPS",
			TrackingNumber: "NEW456",
		})
		require.NoError(t, err)
		assert.Equal(t, "UPS", result.Carrier)
		assert.Equal(t, "NEW456", result.TrackingNumber)
	})

	t.Run("shipment not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, core.ErrNotFound)

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingRequest{
			Carrier: "UPS",
		})
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: uuid.New(),
			Carrier: "FedEx",
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil)
		dbErr := errors.New("update failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).Return(dbErr)

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingRequest{
			Carrier: "UPS",
		})
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_MarkDelivered(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()

		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil).Once()
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(nil)
		updater.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"shipped"}, "delivered").Return(nil)

		now := time.Now()
		delivered := &shipping.Shipment{
			ID:          shipmentID,
			OrderID:     orderID,
			Status:      shipping.StatusDelivered,
			DeliveredAt: &now,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(delivered, nil).Once()

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		require.NoError(t, err)
		assert.Equal(t, shipping.StatusDelivered, result.Status)
		assert.NotNil(t, result.DeliveredAt)
	})

	t.Run("shipment not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, core.ErrNotFound)

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("mark delivered repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: uuid.New(),
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(dbErr)

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("update order status error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, nil, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()
		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil)
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(nil)
		updateErr := errors.New("order update failed")
		updater.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"shipped"}, "delivered").Return(updateErr)

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, updateErr)
	})
}
