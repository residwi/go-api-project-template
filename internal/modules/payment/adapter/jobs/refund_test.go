package jobs

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRefundWorkerCallsSettleRefund(t *testing.T) {
	t.Parallel()

	paymentID, orderID := uuid.New(), uuid.New()
	svc := NewMockRefunder(t)
	svc.EXPECT().SettleRefund(mock.Anything, paymentID, orderID).Return(nil)

	w := NewRefundWorker(svc, time.Minute)
	err := w.Work(t.Context(), &river.Job[RefundArgs]{
		JobRow: &rivertype.JobRow{Kind: "payment.refund"},
		Args:   RefundArgs{PaymentID: paymentID, OrderID: orderID},
	})

	require.NoError(t, err)
}

func TestRefundArgsRouteToThePaymentQueue(t *testing.T) {
	t.Parallel()

	opts := RefundArgs{}.InsertOpts()

	assert.Equal(t, "payment", opts.Queue)
	assert.Equal(t, "payment.refund", RefundArgs{}.Kind())
}
