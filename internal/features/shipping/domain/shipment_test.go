package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanShipOrder(t *testing.T) {
	t.Parallel()

	t.Run("allows a paid order", func(t *testing.T) {
		t.Parallel()
		assert.True(t, CanShipOrder("paid"))
	})

	t.Run("allows an order already in processing", func(t *testing.T) {
		t.Parallel()
		assert.True(t, CanShipOrder("processing"))
	})

	t.Run("refuses an order still awaiting payment", func(t *testing.T) {
		t.Parallel()
		assert.False(t, CanShipOrder("awaiting_payment"))
	})

	t.Run("refuses an order that was cancelled", func(t *testing.T) {
		t.Parallel()
		assert.False(t, CanShipOrder("cancelled"))
	})
}
