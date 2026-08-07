package shipping

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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestService_GetByOrderID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		expected := &Shipment{
			ID:             uuid.New(),
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK999",
			Status:         StatusShipped,
		}
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(expected, nil)

		result, err := svc.GetByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		orderID := uuid.New()
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := svc.GetByOrderID(context.Background(), orderID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_MarkDelivered(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()

		existing := &Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  StatusShipped,
		}
		now := time.Now()
		delivered := &Shipment{
			ID:          shipmentID,
			OrderID:     orderID,
			Status:      StatusDelivered,
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
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		result, err := svc.MarkDelivered(context.Background(), shipmentID)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("mark delivered repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		existing := &Shipment{
			ID:      shipmentID,
			OrderID: uuid.New(),
			Status:  StatusShipped,
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
		t.Parallel()

		repo := NewMockRepository(t)
		orders := NewMockOrderProvider(t)
		updater := NewMockOrderUpdater(t)
		svc := NewService(repo, testhelper.FakeTxRunner{}, orders, updater)

		shipmentID := uuid.New()
		orderID := uuid.New()
		existing := &Shipment{
			ID:      shipmentID,
			OrderID: orderID,
			Status:  StatusShipped,
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
