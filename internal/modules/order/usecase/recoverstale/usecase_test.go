package recoverstale

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// RecoverStaleProcessing had no unit test before this move -- ExpireStale's
// sibling sweep did, but this one did not. Covered here since it now has its
// own slice.
func TestUseCase_RecoverStaleProcessing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("recovers each stale order back to awaiting_payment", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		cmd := New(repo, transition, testhelper.DiscardLogger())

		stale := domain.Order{ID: uuid.New()}
		repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, contract.StaleProcessingThreshold, mock.Anything).
			Return([]domain.Order{stale}, nil)
		transition.EXPECT().Apply(mock.Anything, stale.ID, domain.AwaitingPaymentTransition).Return(nil)

		require.NoError(t, cmd.RecoverStaleProcessing(ctx))
	})

	t.Run("skips an order another worker already moved on", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		cmd := New(repo, transition, testhelper.DiscardLogger())

		stale := domain.Order{ID: uuid.New()}
		repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, contract.StaleProcessingThreshold, mock.Anything).
			Return([]domain.Order{stale}, nil)
		transition.EXPECT().
			Apply(mock.Anything, stale.ID, domain.AwaitingPaymentTransition).
			Return(apperror.ErrConflict)

		require.NoError(t, cmd.RecoverStaleProcessing(ctx))
	})

	t.Run("getting stale orders error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		cmd := New(repo, transition, testhelper.DiscardLogger())

		dbErr := errors.New("database error")
		repo.EXPECT().
			GetStaleProcessingOrders(mock.Anything, contract.StaleProcessingThreshold, mock.Anything).
			Return(nil, dbErr)

		err := cmd.RecoverStaleProcessing(ctx)
		require.ErrorIs(t, err, dbErr)
	})
}
