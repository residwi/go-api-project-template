package shipping

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("creates the shipment and flips the order in one transaction", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)
		shipper := NewMockOrderShipper(t)
		shipper.EXPECT().MarkShipped(mock.Anything, orderID).Return(nil)

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(s *domain.Shipment) bool {
			return s.OrderID == orderID &&
				s.Carrier == "JNE" &&
				s.TrackingNumber == "JP123" &&
				s.Status == domain.StatusShipped
		})).Return(nil)

		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderRead: orders, OrderShip: shipper})
		got, err := svc.Create(t.Context(), orderID, "JNE", "JP123")

		require.NoError(t, err)
		assert.Equal(t, domain.StatusShipped, got.Status)
	})

	t.Run("refuses an order that is not paid or processing", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, Status: "awaiting_payment"}, nil)

		// Repository is never called: the guard runs before the transaction opens.
		svc := New(Deps{
			Repo: NewMockRepository(t), Tx: testhelper.FakeTxRunner{},
			OrderRead: orders, OrderShip: NewMockOrderShipper(t),
		})
		_, err := svc.Create(t.Context(), orderID, "JNE", "JP123")

		require.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("propagates the order lookup failure unchanged", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{}, apperror.ErrNotFound)

		svc := New(Deps{
			Repo: NewMockRepository(t), Tx: testhelper.FakeTxRunner{},
			OrderRead: orders, OrderShip: NewMockOrderShipper(t),
		})
		_, err := svc.Create(t.Context(), orderID, "DHL", "DHL456")

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repository failure prevents the order flip", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)

		dbErr := errors.New("insert failed")
		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Shipment")).Return(dbErr)

		svc := New(Deps{
			Repo: repo, Tx: testhelper.FakeTxRunner{},
			OrderRead: orders, OrderShip: NewMockOrderShipper(t),
		})
		got, err := svc.Create(t.Context(), orderID, "FedEx", "TRACK123")

		assert.Nil(t, got)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("rolls back the shipment when the order flip fails", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()
		flipErr := errors.New("order is no longer shippable")

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: uuid.New(), Status: "paid"}, nil)
		shipper := NewMockOrderShipper(t)
		shipper.EXPECT().MarkShipped(mock.Anything, orderID).Return(flipErr)

		repo := NewMockRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Shipment")).Return(nil)

		// testhelper.FakeTxRunner runs the callback inline and returns its error, so
		// a non-nil return here is what rollback looks like from this level. The
		// real rollback is covered in adapter/postgres/repository_test.go against a
		// live transaction.
		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderRead: orders, OrderShip: shipper})
		got, err := svc.Create(t.Context(), orderID, "FedEx", "TRACK123")

		assert.Nil(t, got)
		require.ErrorIs(t, err, flipErr)
	})
}

func TestService_Deliver(t *testing.T) {
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

		deliverer := NewMockOrderDeliverer(t)
		deliverer.EXPECT().MarkDelivered(mock.Anything, orderID).Return(nil)

		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderDeliver: deliverer})
		got, err := svc.Deliver(t.Context(), shipmentID)

		require.NoError(t, err)
		assert.Equal(t, domain.StatusDelivered, got.Status)
		assert.NotNil(t, got.DeliveredAt)
	})

	t.Run("propagates a missing shipment before opening a transaction", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderDeliver: NewMockOrderDeliverer(t)})
		_, err := svc.Deliver(t.Context(), shipmentID)

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

		// No EXPECT() on deliverer at all: the mock's own strictness is what proves
		// deliverer.MarkDelivered is never called on this path, not just that the
		// error we assert on happens to match.
		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderDeliver: NewMockOrderDeliverer(t)})
		_, err := svc.Deliver(t.Context(), shipmentID)

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

		deliverer := NewMockOrderDeliverer(t)
		deliverer.EXPECT().MarkDelivered(mock.Anything, orderID).Return(flipErr)

		svc := New(Deps{Repo: repo, Tx: testhelper.FakeTxRunner{}, OrderDeliver: deliverer})
		_, err := svc.Deliver(t.Context(), shipmentID)

		require.ErrorIs(t, err, flipErr)
	})
}

func TestService_UpdateTracking(t *testing.T) {
	t.Parallel()

	t.Run("replaces both fields when both are given", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, Carrier: "JNE", TrackingNumber: "OLD"}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(s *domain.Shipment) bool {
			return s.Carrier == "SiCepat" && s.TrackingNumber == "NEW"
		})).Return(nil)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, Carrier: "SiCepat", TrackingNumber: "NEW"}, nil).Once()

		svc := New(Deps{Repo: repo})
		got, err := svc.UpdateTracking(t.Context(), shipmentID, "SiCepat", "NEW")

		require.NoError(t, err)
		assert.Equal(t, "SiCepat", got.Carrier)
	})

	t.Run("leaves a field untouched when its value is empty", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, Carrier: "JNE", TrackingNumber: "KEEP"}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(s *domain.Shipment) bool {
			// An empty carrier must not blank the stored one.
			return s.Carrier == "JNE" && s.TrackingNumber == "NEW"
		})).Return(nil)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).
			Return(&domain.Shipment{ID: shipmentID, Carrier: "JNE", TrackingNumber: "NEW"}, nil).Once()

		svc := New(Deps{Repo: repo})
		got, err := svc.UpdateTracking(t.Context(), shipmentID, "", "NEW")

		require.NoError(t, err)
		assert.Equal(t, "JNE", got.Carrier)
	})

	t.Run("propagates a missing shipment", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		svc := New(Deps{Repo: repo})
		_, err := svc.UpdateTracking(t.Context(), shipmentID, "JNE", "")

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_GetForUser(t *testing.T) {
	t.Parallel()

	t.Run("returns the shipment when the order belongs to the caller", func(t *testing.T) {
		t.Parallel()
		userID, orderID, shipmentID := uuid.New(), uuid.New(), uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: userID, Status: "shipped"}, nil)

		repo := NewMockRepository(t)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).
			Return(&domain.Shipment{ID: shipmentID, OrderID: orderID, Status: domain.StatusShipped}, nil)

		svc := New(Deps{Repo: repo, OrderRead: orders})
		got, err := svc.GetForUser(t.Context(), userID, orderID)

		require.NoError(t, err)
		assert.Equal(t, &domain.Shipment{ID: shipmentID, OrderID: orderID, Status: domain.StatusShipped}, got)
	})

	t.Run("reports not found when the order belongs to someone else", func(t *testing.T) {
		t.Parallel()
		userID, orderID := uuid.New(), uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: uuid.New(), Status: "shipped"}, nil)

		// Not ErrForbidden: a 403 would confirm the order exists to someone who
		// does not own it.
		svc := New(Deps{Repo: NewMockRepository(t), OrderRead: orders})
		_, err := svc.GetForUser(t.Context(), userID, orderID)

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("propagates the order lookup failure unchanged", func(t *testing.T) {
		t.Parallel()
		orderID := uuid.New()

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{}, apperror.ErrNotFound)

		svc := New(Deps{Repo: NewMockRepository(t), OrderRead: orders})
		_, err := svc.GetForUser(t.Context(), uuid.New(), orderID)

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("propagates the shipment lookup failure unchanged when ownership checks out", func(t *testing.T) {
		t.Parallel()
		userID, orderID := uuid.New(), uuid.New()
		repoErr := errors.New("connection reset")

		orders := NewMockOrderGetter(t)
		orders.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{ID: orderID, UserID: userID, Status: "shipped"}, nil)

		repo := NewMockRepository(t)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, repoErr)

		svc := New(Deps{Repo: repo, OrderRead: orders})
		_, err := svc.GetForUser(t.Context(), userID, orderID)

		require.ErrorIs(t, err, repoErr)
	})
}
