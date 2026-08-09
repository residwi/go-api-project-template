package changestatus

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success valid transition paid to processing", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusPaid}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().UpdateStatus(mock.Anything, orderID, domain.StatusPaid, domain.StatusProcessing).Return(nil)

		err := cmd.Execute(ctx, orderID, domain.StatusProcessing)

		assert.NoError(t, err)
	})

	t.Run("success valid transition processing to shipped", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusProcessing}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().
			UpdateStatus(mock.Anything, orderID, domain.StatusProcessing, domain.StatusShipped).
			Return(nil)

		err := cmd.Execute(ctx, orderID, domain.StatusShipped)

		assert.NoError(t, err)
	})

	t.Run("invalid transition awaiting_payment to delivered", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusAwaitingPayment}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := cmd.Execute(ctx, orderID, domain.StatusDelivered)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("rejects managed status cancelled without a direct write", func(t *testing.T) {
		t.Parallel()

		// These unwind inventory or money, so Execute rejects them before any
		// lookup rather than writing the status bare.
		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		err := cmd.Execute(ctx, orderID, domain.StatusCancelled)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("rejects managed status refunded without a direct write", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		err := cmd.Execute(ctx, orderID, domain.StatusRefunded)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		err := cmd.Execute(ctx, orderID, domain.StatusProcessing)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update status repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionPort(t)
		cmd := New(repo, transition)

		existingOrder := &domain.Order{ID: orderID, Status: domain.StatusPaid}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().
			UpdateStatus(mock.Anything, orderID, domain.StatusPaid, domain.StatusProcessing).
			Return(apperror.ErrConflict)

		err := cmd.Execute(ctx, orderID, domain.StatusProcessing)

		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}
