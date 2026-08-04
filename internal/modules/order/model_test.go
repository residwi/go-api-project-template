package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTransitionStockFlags pins which transitions change an order's persisted
// stock state: paying an order deducts its reserved stock, and cancelling,
// expiring, or refunding reverses the inventory hold. The flags ride along in the
// status compare-and-set, so a later reversal can choose release vs restock vs
// no-op from the persisted facts rather than inferring from a status — which it
// cannot, since fulfillment_failed is reachable from both reserved-only and
// deducted states.
func TestTransitionStockFlags(t *testing.T) {
	t.Run("PaidTransition marks stock deducted", func(t *testing.T) {
		assert.True(t, PaidTransition.SetsStockDeducted)
		assert.False(t, PaidTransition.SetsStockReversed)
	})

	t.Run("cancel, expire, and refund mark the hold reversed", func(t *testing.T) {
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
