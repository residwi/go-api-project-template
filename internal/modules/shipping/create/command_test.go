package create

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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("creates the shipment and flips the order in one transaction", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)
		orders.EXPECT().MarkShipped(mock.Anything, orderID).Return(nil)

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(s *domain.Shipment) bool {
			return s.OrderID == orderID &&
				s.Carrier == "JNE" &&
				s.TrackingNumber == "JP123" &&
				s.Status == domain.StatusShipped
		})).Return(nil)

		got, err := New(repo, testhelper.FakeTxRunner{}, orders).
			Execute(context.Background(), orderID, Params{Carrier: "JNE", TrackingNumber: "JP123"})

		require.NoError(t, err)
		assert.Equal(t, domain.StatusShipped, got.Status)
	})

	t.Run("refuses an order that is not paid or processing", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, Status: "awaiting_payment"}, nil)

		// Repository is never called: the guard runs before the transaction opens.
		_, err := New(NewMockRepository(t), testhelper.FakeTxRunner{}, orders).
			Execute(context.Background(), orderID, Params{Carrier: "JNE", TrackingNumber: "JP123"})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("propagates the order lookup failure unchanged", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{}, apperror.ErrNotFound)

		_, err := New(NewMockRepository(t), testhelper.FakeTxRunner{}, orders).
			Execute(context.Background(), orderID, Params{Carrier: "DHL", TrackingNumber: "DHL456"})

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repository failure prevents the order flip", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)

		dbErr := errors.New("insert failed")
		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Shipment")).Return(dbErr)

		got, err := New(repo, testhelper.FakeTxRunner{}, orders).
			Execute(context.Background(), orderID, Params{Carrier: "FedEx", TrackingNumber: "TRACK123"})

		assert.Nil(t, got)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("rolls back the shipment when the order flip fails", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()
		flipErr := errors.New("order is no longer shippable")

		orders := NewMockOrderPort(t)
		orders.EXPECT().GetInfo(mock.Anything, orderID).
			Return(ordercontract.Order{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)
		orders.EXPECT().MarkShipped(mock.Anything, orderID).Return(flipErr)

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Shipment")).Return(nil)

		// testhelper.FakeTxRunner runs the callback inline and returns its error, so
		// a non-nil return here is what rollback looks like from this level. The
		// real rollback is covered in create/postgres/repository_test.go against a
		// live transaction.
		got, err := New(repo, testhelper.FakeTxRunner{}, orders).
			Execute(context.Background(), orderID, Params{Carrier: "FedEx", TrackingNumber: "TRACK123"})

		assert.Nil(t, got)
		require.ErrorIs(t, err, flipErr)
	})
}
