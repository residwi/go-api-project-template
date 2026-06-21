package order_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/features/order"
)

// allStatuses is every order status, used to exhaustively probe CanTransition.
var allStatuses = []order.Status{
	order.StatusAwaitingPayment,
	order.StatusPaymentProcessing,
	order.StatusPaid,
	order.StatusProcessing,
	order.StatusShipped,
	order.StatusDelivered,
	order.StatusCancelled,
	order.StatusExpired,
	order.StatusRefunded,
	order.StatusFulfillmentFailed,
}

// TestCanTransition_Graph locks down the entire state machine derived from the
// named transitions in transition.go. Editing any Transition's From/To set
// changes which (from,to) pairs are legal, and this test will catch it — every
// pair not listed here must be rejected.
func TestCanTransition_Graph(t *testing.T) {
	// allowed[from] is the exact set of statuses `from` may transition to.
	allowed := map[order.Status][]order.Status{
		order.StatusAwaitingPayment: {
			order.StatusPaymentProcessing, order.StatusPaid,
			order.StatusFulfillmentFailed, order.StatusCancelled, order.StatusExpired,
		},
		order.StatusPaymentProcessing: {
			order.StatusPaymentProcessing, order.StatusAwaitingPayment, order.StatusPaid,
			order.StatusFulfillmentFailed, order.StatusCancelled,
		},
		order.StatusPaid: {
			order.StatusFulfillmentFailed, order.StatusRefunded,
			order.StatusShipped, order.StatusProcessing,
		},
		order.StatusProcessing:        {order.StatusShipped, order.StatusRefunded},
		order.StatusShipped:           {order.StatusDelivered, order.StatusRefunded},
		order.StatusDelivered:         {order.StatusRefunded},
		order.StatusCancelled:         {order.StatusFulfillmentFailed},
		order.StatusExpired:           {order.StatusFulfillmentFailed},
		order.StatusRefunded:          {},
		order.StatusFulfillmentFailed: {order.StatusRefunded, order.StatusCancelled},
	}

	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := slices.Contains(allowed[from], to)
			assert.Equalf(t, want, order.CanTransition(from, to),
				"CanTransition(%s, %s) should be %v", from, to, want)
		}
	}
}
