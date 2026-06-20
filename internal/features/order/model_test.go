package order_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/features/order"
)

// TestOrder_StockDeducted pins the rule that decides, when an order is reversed,
// whether its inventory must be restocked (stock already left the shelf) or
// merely released (stock was only reserved). Stock is deducted when the order
// becomes paid, so every status reachable only after that has deducted stock.
func TestOrder_StockDeducted(t *testing.T) {
	t.Run("post-payment statuses have stock deducted", func(t *testing.T) {
		deducted := []order.Status{
			order.StatusPaid,
			order.StatusProcessing,
			order.StatusShipped,
			order.StatusDelivered,
		}
		for _, s := range deducted {
			assert.Truef(t, order.Order{Status: s}.StockDeducted(), "status %q", s)
		}
	})

	t.Run("pre-payment and ambiguous terminal statuses do not", func(t *testing.T) {
		notDeducted := []order.Status{
			order.StatusAwaitingPayment,
			order.StatusPaymentProcessing,
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
