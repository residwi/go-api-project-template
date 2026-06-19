package order_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/features/order"
)

// TestOrder_StockDeducted pins the rule that decides, when an order is reversed,
// whether its inventory must be restocked (stock already left the shelf) or
// merely released (stock was only reserved). It mirrors the behaviour payment
// previously inlined: deducted iff paid or delivered.
func TestOrder_StockDeducted(t *testing.T) {
	t.Run("paid and delivered have stock deducted", func(t *testing.T) {
		assert.True(t, order.Order{Status: order.StatusPaid}.StockDeducted())
		assert.True(t, order.Order{Status: order.StatusDelivered}.StockDeducted())
	})

	t.Run("pre-payment and terminal statuses do not", func(t *testing.T) {
		notDeducted := []order.Status{
			order.StatusAwaitingPayment,
			order.StatusPaymentProcessing,
			order.StatusProcessing,
			order.StatusShipped,
			order.StatusCancelled,
			order.StatusExpired,
			order.StatusRefunded,
			order.StatusFulfillmentFailed,
		}
		for _, s := range notDeducted {
			assert.Falsef(t, order.Order{Status: s}.StockDeducted(), "status %q", s)
		}
	})
}
