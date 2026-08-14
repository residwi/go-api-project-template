package updatetracking

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

func TestCommandExecute(t *testing.T) {
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

		got, err := New(repo).Execute(context.Background(), shipmentID,
			Params{Carrier: "SiCepat", TrackingNumber: "NEW"})

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

		got, err := New(repo).Execute(context.Background(), shipmentID, Params{TrackingNumber: "NEW"})

		require.NoError(t, err)
		assert.Equal(t, "JNE", got.Carrier)
	})

	t.Run("propagates a missing shipment", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		repo := NewMockRepository(t)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		_, err := New(repo).Execute(context.Background(), shipmentID, Params{Carrier: "JNE"})

		require.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
