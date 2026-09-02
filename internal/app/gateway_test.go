package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/features/payment"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/stripe"
)

// TestNewGateway pins each Config.Gateway string to the concrete
// implementation it must build: a typo in either case string here or in
// app.go's switch fails this test, where LoadConfig's own validation
// (config_test.go) cannot see it -- LoadConfig only checks the string is one
// of the three, not that this switch still routes each one correctly.
func TestNewGateway(t *testing.T) {
	t.Run("stripe", func(t *testing.T) {
		gw := newPaymentGateway(payment.Config{Gateway: payment.GatewayStripe, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaystripe.Gateway{}, gw)
	})

	t.Run("midtrans", func(t *testing.T) {
		gw := newPaymentGateway(payment.Config{Gateway: payment.GatewayMidtrans, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaymidtrans.Gateway{}, gw)
	})

	t.Run("mock", func(t *testing.T) {
		gw := newPaymentGateway(payment.Config{Gateway: payment.GatewayMock, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaymock.Gateway{}, gw)
	})
}
