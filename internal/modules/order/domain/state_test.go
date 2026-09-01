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

var allTransitions = []Transition{
	ToAwaitingPayment,
	ToPaymentProcessing,
	ToPaymentAttempt,
	ToPaid,
	ToProcessing,
	ToShipped,
	ToDelivered,
	ToFulfillmentFailedAfterCharge,
	ToFulfillmentFailedCompensating,
	ToCancelled,
	ToExpired,
	ToRefunded,
}

func TestTransitionGraph(t *testing.T) {
	t.Parallel()

	t.Run("the transitions describe exactly this graph", func(t *testing.T) {
		t.Parallel()

		want := map[Status][]Status{
			StatusAwaitingPayment: {
				StatusCancelled,
				StatusExpired,
				StatusFulfillmentFailed,
				StatusPaid,
				StatusPaymentProcessing,
			},
			StatusPaymentProcessing: {
				StatusAwaitingPayment,
				StatusCancelled,
				StatusFulfillmentFailed,
				StatusPaid,
				StatusPaymentProcessing,
			},
			StatusPaid: {
				StatusFulfillmentFailed,
				StatusProcessing,
				StatusRefunded,
				StatusShipped,
			},
			StatusProcessing:        {StatusRefunded, StatusShipped},
			StatusShipped:           {StatusDelivered, StatusRefunded},
			StatusDelivered:         {StatusRefunded},
			StatusCancelled:         {StatusFulfillmentFailed},
			StatusExpired:           {StatusFulfillmentFailed},
			StatusFulfillmentFailed: {StatusCancelled, StatusRefunded},
			StatusRefunded:          nil,
		}

		assert.Equal(t, want, forwardGraph())
	})

	t.Run("a payment attempt refuses an order already processing", func(t *testing.T) {
		t.Parallel()

		assert.NotContains(t, ToPaymentAttempt.From, StatusPaymentProcessing)
		assert.Contains(t, ToPaymentProcessing.From, StatusPaymentProcessing)
	})

	t.Run("failing after a charge refuses an order never charged", func(t *testing.T) {
		t.Parallel()

		assert.NotContains(t, ToFulfillmentFailedAfterCharge.From, StatusAwaitingPayment)
		assert.Contains(t, ToFulfillmentFailedCompensating.From, StatusAwaitingPayment)
	})
}

func TestTransitionStockEffect(t *testing.T) {
	t.Parallel()

	t.Run("paying deducts the stock", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ToPaid.DeductsStock())
		assert.False(t, ToPaid.ReversesStock())
	})

	t.Run("cancel, expire, and refund reverse the hold", func(t *testing.T) {
		t.Parallel()

		for _, tr := range []Transition{ToCancelled, ToExpired, ToRefunded} {
			assert.Truef(t, tr.ReversesStock(), "transition to %q", tr.To)
			assert.Falsef(t, tr.DeductsStock(), "transition to %q", tr.To)
		}
	})

	t.Run("every other transition leaves the stock alone", func(t *testing.T) {
		t.Parallel()

		for _, tr := range []Transition{
			ToAwaitingPayment,
			ToPaymentProcessing,
			ToPaymentAttempt,
			ToProcessing,
			ToShipped,
			ToDelivered,
			ToFulfillmentFailedAfterCharge,
			ToFulfillmentFailedCompensating,
		} {
			assert.Falsef(t, tr.DeductsStock(), "transition to %q", tr.To)
			assert.Falsef(t, tr.ReversesStock(), "transition to %q", tr.To)
		}
	})
}

func forwardGraph() map[Status][]Status {
	graph := make(map[Status][]Status, len(allStatuses))
	for _, s := range allStatuses {
		graph[s] = nil
	}
	for _, tr := range allTransitions {
		for _, from := range tr.From {
			if !slices.Contains(graph[from], tr.To) {
				graph[from] = append(graph[from], tr.To)
			}
		}
	}
	for _, tos := range graph {
		slices.Sort(tos)
	}
	return graph
}
