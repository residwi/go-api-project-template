package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransitionStockFlags(t *testing.T) {
	t.Parallel()

	t.Run("PaidTransition marks stock deducted", func(t *testing.T) {
		t.Parallel()

		assert.True(t, PaidTransition.SetsStockDeducted)
		assert.False(t, PaidTransition.SetsStockReversed)
	})

	t.Run("cancel, expire, and refund mark the hold reversed", func(t *testing.T) {
		t.Parallel()

		for _, tr := range []Transition{
			CancelledTransition,
			ExpiredTransition,
			RefundTransition,
		} {
			assert.Truef(t, tr.SetsStockReversed, "transition to %q", tr.To)
			assert.Falsef(t, tr.SetsStockDeducted, "transition to %q", tr.To)
		}
	})

	t.Run("other transitions touch neither stock flag", func(t *testing.T) {
		t.Parallel()

		for _, tr := range []Transition{
			PaymentProcessingTransition,
			AwaitingPaymentTransition,
			ShippedTransition,
			DeliveredTransition,
			ProcessingTransition,
			FulfillmentFailedAfterChargeTransition,
			FulfillmentFailedCompensatingTransition,
		} {
			assert.Falsef(t, tr.SetsStockDeducted, "transition to %q", tr.To)
			assert.Falsef(t, tr.SetsStockReversed, "transition to %q", tr.To)
		}
	})
}

func TestOrderDispatched(t *testing.T) {
	t.Parallel()

	t.Run("is true once the order has shipped", func(t *testing.T) {
		t.Parallel()

		assert.True(t, (&Order{Status: StatusShipped}).Dispatched())
	})

	t.Run("is true once the order is delivered", func(t *testing.T) {
		t.Parallel()

		assert.True(t, (&Order{Status: StatusDelivered}).Dispatched())
	})

	t.Run("is false while the order is only paid", func(t *testing.T) {
		t.Parallel()

		assert.False(t, (&Order{Status: StatusPaid}).Dispatched())
	})
}
