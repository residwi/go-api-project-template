package transition

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

func TestApplier_Apply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("forwards the transition's to/from to the compare-and-set primitive", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		a := New(repo)
		repo.EXPECT().Apply(mock.Anything, orderID, domain.PaidTransition).Return(nil)

		err := a.Apply(ctx, orderID, domain.PaidTransition)

		assert.NoError(t, err)
	})

	t.Run("conflict error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		a := New(repo)
		repo.EXPECT().Apply(mock.Anything, orderID, domain.RefundTransition).Return(apperror.ErrConflict)

		err := a.Apply(ctx, orderID, domain.RefundTransition)

		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestApplier_UpdateStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("forwards from and to to the dynamic compare-and-set", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		a := New(repo)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, domain.StatusPaid, domain.StatusProcessing).Return(nil)

		err := a.UpdateStatus(ctx, orderID, domain.StatusPaid, domain.StatusProcessing)

		require.NoError(t, err)
	})

	t.Run("conflict error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		a := New(repo)
		repo.EXPECT().
			UpdateStatus(mock.Anything, orderID, domain.StatusPaid, domain.StatusProcessing).
			Return(apperror.ErrConflict)

		err := a.UpdateStatus(ctx, orderID, domain.StatusPaid, domain.StatusProcessing)

		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

// TestApplierMarkPaid stands in for all eight Mark* methods: each is a
// one-line forward to Apply with its named Transition, so proving the wiring
// for one proves the pattern -- the allowed-from set itself is
// domain/transition_test.go's TestCanTransition_Graph.
func TestApplierMarkPaid(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()

	repo := NewMockRepository(t)
	a := New(repo)
	repo.EXPECT().Apply(mock.Anything, orderID, domain.PaidTransition).Return(nil)

	require.NoError(t, a.MarkPaid(context.Background(), orderID))
}
