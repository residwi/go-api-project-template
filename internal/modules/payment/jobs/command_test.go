package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Process(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("unknown action returns error", func(t *testing.T) {
		t.Parallel()

		cmd, _, charge, refund := newTestCommand(t)

		job := domain.Job{
			ID:     uuid.New(),
			Action: "invalid_action",
		}

		processErr := cmd.Process(ctx, job)
		require.Error(t, processErr)
		charge.AssertNotCalled(t, "ProcessCharge", mock.Anything, mock.Anything)
		refund.AssertNotCalled(t, "ProcessRefund", mock.Anything, mock.Anything)
	})

	t.Run("charge action dispatches to ChargeProcessor", func(t *testing.T) {
		t.Parallel()

		cmd, _, charge, _ := newTestCommand(t)

		job := domain.Job{ID: uuid.New(), Action: domain.ActionCharge}

		charge.EXPECT().ProcessCharge(mock.Anything, job).Return(errors.New("charge failed"))

		processErr := cmd.Process(ctx, job)
		assert.EqualError(t, processErr, "charge failed")
	})

	t.Run("refund action dispatches to RefundProcessor", func(t *testing.T) {
		t.Parallel()

		cmd, _, _, refund := newTestCommand(t)

		job := domain.Job{ID: uuid.New(), Action: domain.ActionRefund}

		refund.EXPECT().ProcessRefund(mock.Anything, job).Return(nil)

		processErr := cmd.Process(ctx, job)
		assert.NoError(t, processErr)
	})
}

func TestCommand_CancelPendingByOrderID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _ := newTestCommand(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		err := cmd.CancelPendingByOrderID(ctx, orderID)

		require.NoError(t, err)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _ := newTestCommand(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(errors.New("db error"))

		err := cmd.CancelPendingByOrderID(ctx, orderID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})
}

func newTestCommand(t *testing.T) (*Command, *MockRepository, *MockChargeProcessor, *MockRefundProcessor) {
	repo := NewMockRepository(t)
	charge := NewMockChargeProcessor(t)
	refund := NewMockRefundProcessor(t)

	cmd := New(repo, testhelper.DiscardLogger())
	cmd.SetProcessors(charge, refund)

	return cmd, repo, charge, refund
}
