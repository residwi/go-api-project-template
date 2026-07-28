package shipping_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/shipping"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	mocks "github.com/residwi/go-api-project-template/mocks/shipping"
)

func TestService_CreateShipment(t *testing.T) {
	t.Run("success order in paid status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

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

		updater.EXPECT().MarkShipped(mock.Anything, orderID).
			Return(nil)

		ctx := context.Background()
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateParams{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
		})
		require.NoError(t, err)
		expected := &shipping.Shipment{
			ID:             shippedID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusShipped,
			ShippedAt:      &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		assert.Equal(t, expected, result)
	})

	t.Run("order wrong status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).
			Return(shipping.OrderInfo{
				ID:     orderID,
				Status: "pending",
			}, nil)

		_, err := svc.CreateShipment(context.Background(), orderID, shipping.CreateParams{
			Carrier:        "UPS",
			TrackingNumber: "UPS123",
		})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("order not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).
			Return(shipping.OrderInfo{}, apperror.ErrNotFound)

		_, err := svc.CreateShipment(context.Background(), orderID, shipping.CreateParams{
			Carrier:        "DHL",
			TrackingNumber: "DHL456",
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo create error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		orders.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID:     orderID,
			UserID: uuid.New(),
			Status: "paid",
		}, nil)

		dbErr := errors.New("insert failed")
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*shipping.Shipment")).Return(dbErr)

		ctx := context.Background()
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateParams{
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
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

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
		updater.EXPECT().MarkShipped(mock.Anything, orderID).Return(updateErr)

		ctx := context.Background()
		result, err := svc.CreateShipment(ctx, orderID, shipping.CreateParams{
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
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

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
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := svc.GetByOrderID(context.Background(), orderID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_UpdateTracking(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

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

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingParams{
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
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingParams{
			Carrier: "UPS",
		})
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

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

		result, err := svc.UpdateTracking(context.Background(), shipmentID, shipping.UpdateTrackingParams{
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
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()

		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  shipping.StatusShipped,
		}
		now := time.Now()
		delivered := &shipping.Shipment{
			ID:          shipmentID,
			OrderID:     orderID,
			Status:      shipping.StatusDelivered,
			DeliveredAt: &now,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil).Once()
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(delivered, nil)
		updater.EXPECT().MarkDelivered(mock.Anything, orderID).Return(nil)

		ctx := context.Background()
		result, err := svc.MarkDelivered(ctx, shipmentID)
		require.NoError(t, err)
		assert.Equal(t, delivered, result)
	})

	t.Run("shipment not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("mark delivered repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: uuid.New(),
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil)
		dbErr := errors.New("database error")
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(nil, dbErr)

		ctx := context.Background()
		result, err := svc.MarkDelivered(ctx, shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("update order status error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		orders := mocks.NewMockOrderProvider(t)
		updater := mocks.NewMockOrderUpdater(t)
		svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()
		existing := &shipping.Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  shipping.StatusShipped,
		}
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(existing, nil)
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(existing, nil)
		updateErr := errors.New("order update failed")
		updater.EXPECT().MarkDelivered(mock.Anything, orderID).Return(updateErr)

		ctx := context.Background()
		result, err := svc.MarkDelivered(ctx, shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, updateErr)
	})
}
