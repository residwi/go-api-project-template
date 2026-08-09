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

func TestDispatcher_Process(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("unknown action returns error", func(t *testing.T) {
		t.Parallel()

		d, charge, refund := newTestDispatcher(t)

		job := domain.Job{
			ID:     uuid.New(),
			Action: "invalid_action",
		}

		processErr := d.Process(ctx, job)
		require.Error(t, processErr)
		charge.AssertNotCalled(t, "ProcessCharge", mock.Anything, mock.Anything)
		refund.AssertNotCalled(t, "ProcessRefund", mock.Anything, mock.Anything)
	})

	t.Run("charge action dispatches to ChargeProcessor", func(t *testing.T) {
		t.Parallel()

		d, charge, _ := newTestDispatcher(t)

		job := domain.Job{ID: uuid.New(), Action: domain.ActionCharge}

		charge.EXPECT().ProcessCharge(mock.Anything, job).Return(errors.New("charge failed"))

		processErr := d.Process(ctx, job)
		assert.EqualError(t, processErr, "charge failed")
	})

	t.Run("refund action dispatches to RefundProcessor", func(t *testing.T) {
		t.Parallel()

		d, _, refund := newTestDispatcher(t)

		job := domain.Job{ID: uuid.New(), Action: domain.ActionRefund}

		refund.EXPECT().ProcessRefund(mock.Anything, job).Return(nil)

		processErr := d.Process(ctx, job)
		assert.NoError(t, processErr)
	})
}

func newTestDispatcher(t *testing.T) (*Dispatcher, *MockChargeProcessor, *MockRefundProcessor) {
	charge := NewMockChargeProcessor(t)
	refund := NewMockRefundProcessor(t)

	d := NewDispatcher(charge, refund, testhelper.DiscardLogger())

	return d, charge, refund
}
