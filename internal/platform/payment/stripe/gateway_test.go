package stripe

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/payment"
)

func TestNew(t *testing.T) {
	gw := New("test-key", 30*time.Second)
	require.NotNil(t, gw)
}

func TestGateway_Charge(t *testing.T) {
	gw := New("test-key", 30*time.Second)
	_, err := gw.Charge(context.Background(), payment.ChargeRequest{})
	assert.EqualError(t, err, "stripe: not implemented")
}

func TestGateway_Refund(t *testing.T) {
	gw := New("test-key", 30*time.Second)
	_, err := gw.Refund(context.Background(), payment.RefundRequest{})
	assert.EqualError(t, err, "stripe: not implemented")
}
