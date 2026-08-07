package deliver

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
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("marks the shipment and the order delivered in one transaction", func(t *testing.T) {
		t.Parallel()

		shipmentID, orderID := uuid.New(), uuid.New()
		deliveredAt := time.Now()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID, Status: domain.StatusShipped}, nil)
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).
			Return(&domain.Shipment{
				ID: shipmentID, OrderID: orderID,
				Status: domain.StatusDelivered, DeliveredAt: &deliveredAt,
			}, nil)

		orders := NewMockOrderPort(t)
		orders.EXPECT().MarkDelivered(mock.Anything, orderID).Return(nil)

		got, err := New(repo, testhelper.FakeTxRunner{}, orders).Execute(context.Background(), shipmentID)

		require.NoError(t, err)
		assert.Equal(t, domain.StatusDelivered, got.Status)
		assert.NotNil(t, got.DeliveredAt)
	})

	t.Run("propagates a missing shipment before opening a transaction", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		_, err := New(repo, testhelper.FakeTxRunner{}, NewMockOrderPort(t)).Execute(context.Background(), shipmentID)

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("never flips the order when marking the shipment delivered fails", func(t *testing.T) {
		t.Parallel()

		shipmentID, orderID := uuid.New(), uuid.New()
		markErr := errors.New("update failed")

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID}, nil)
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(nil, markErr)

		// No EXPECT() on orders at all: the mock's own strictness is what proves
		// orders.MarkDelivered is never called on this path, not just that the
		// error we assert on happens to match.
		_, err := New(repo, testhelper.FakeTxRunner{}, NewMockOrderPort(t)).Execute(context.Background(), shipmentID)

		require.ErrorIs(t, err, markErr)
	})

	t.Run("rolls back when the order flip fails", func(t *testing.T) {
		t.Parallel()

		shipmentID, orderID := uuid.New(), uuid.New()
		flipErr := errors.New("order not shipped")

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID}, nil)
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID}, nil)

		orders := NewMockOrderPort(t)
		orders.EXPECT().MarkDelivered(mock.Anything, orderID).Return(flipErr)

		_, err := New(repo, testhelper.FakeTxRunner{}, orders).Execute(context.Background(), shipmentID)

		require.ErrorIs(t, err, flipErr)
	})
}
