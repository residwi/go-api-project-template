package order

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

// TestCanTransition_Graph locks down the entire state machine derived from the
// named transitions in transition.go. Editing any Transition's From/To set
// changes which (from,to) pairs are legal, and this test will catch it — every
// pair not listed here must be rejected.
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
