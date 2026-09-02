package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestAddress_ZeroValue(t *testing.T) {
	t.Parallel()

	var addr Address
	assert.Equal(t, Address{}, addr)
}
