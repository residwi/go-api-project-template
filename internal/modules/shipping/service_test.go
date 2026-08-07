package shipping

import (
	"context"
	"testing"

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
