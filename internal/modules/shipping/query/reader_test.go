package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

func TestReaderGetByOrderIDForUser(t *testing.T) {
	t.Parallel()

	t.Run("returns the shipment when the order belongs to the caller", func(t *testing.T) {
		t.Parallel()
		userID, orderID, shipmentID := uuid.New(), uuid.New(), uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: userID, Status: "shipped"}, nil)

		repo := NewMockRepository(t)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID, Status: domain.StatusShipped}, nil)

		got, err := New(repo, orders).GetByOrderIDForUser(context.Background(), userID, orderID)

		require.NoError(t, err)
		assert.Equal(t, &domain.Shipment{ID: shipmentID, OrderID: orderID, Status: domain.StatusShipped}, got)
	})

	t.Run("reports not found when the order belongs to someone else", func(t *testing.T) {
		t.Parallel()
		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: uuid.New(), Status: "shipped"}, nil)

		// Not ErrForbidden: a 403 would confirm the order exists to someone who
		// does not own it.
		_, err := New(NewMockRepository(t), orders).GetByOrderIDForUser(context.Background(), userID, orderID)

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("propagates the order lookup failure unchanged", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{}, apperror.ErrNotFound)

		_, err := New(NewMockRepository(t), orders).GetByOrderIDForUser(context.Background(), uuid.New(), orderID)

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("propagates the shipment lookup failure unchanged when ownership checks out", func(t *testing.T) {
		t.Parallel()
		userID, orderID := uuid.New(), uuid.New()
		repoErr := errors.New("connection reset")

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: userID, Status: "shipped"}, nil)

		repo := NewMockRepository(t)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, repoErr)

		_, err := New(repo, orders).GetByOrderIDForUser(context.Background(), userID, orderID)

		require.ErrorIs(t, err, repoErr)
	})
}
