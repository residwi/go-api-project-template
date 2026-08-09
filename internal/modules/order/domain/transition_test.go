package domain

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

var allStatuses = []Status{
	StatusAwaitingPayment,
	StatusPaymentProcessing,
	StatusPaid,
	StatusProcessing,
	StatusShipped,
	StatusDelivered,
	StatusCancelled,
	StatusExpired,
	StatusRefunded,
	StatusFulfillmentFailed,
}

func TestCanTransition_Graph(t *testing.T) {
	t.Parallel()

	allowed := map[Status][]Status{
		StatusAwaitingPayment: {
			StatusPaymentProcessing, StatusPaid,
			StatusFulfillmentFailed, StatusCancelled, StatusExpired,
		},
		StatusPaymentProcessing: {
			StatusPaymentProcessing, StatusAwaitingPayment, StatusPaid,
			StatusFulfillmentFailed, StatusCancelled,
		},
		StatusPaid: {
			StatusFulfillmentFailed, StatusRefunded,
			StatusShipped, StatusProcessing,
		},
		StatusProcessing:        {StatusShipped, StatusRefunded},
		StatusShipped:           {StatusDelivered, StatusRefunded},
		StatusDelivered:         {StatusRefunded},
		StatusCancelled:         {StatusFulfillmentFailed},
		StatusExpired:           {StatusFulfillmentFailed},
		StatusRefunded:          {},
		StatusFulfillmentFailed: {StatusRefunded, StatusCancelled},
	}

	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := slices.Contains(allowed[from], to)
			assert.Equalf(t, want, CanTransition(from, to),
				"CanTransition(%s, %s) should be %v", from, to, want)
		}
	}
}

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
