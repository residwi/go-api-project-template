package midtrans

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
)

func TestNew(t *testing.T) {
	t.Parallel()

	gw := New("test-key", 30*time.Second)
	require.NotNil(t, gw)
}

func TestGateway_Charge(t *testing.T) {
	t.Parallel()

	gw := New("test-key", 30*time.Second)
	_, err := gw.Charge(context.Background(), gateway.ChargeRequest{})
	assert.EqualError(t, err, "midtrans: not implemented")
}

func TestGateway_Refund(t *testing.T) {
	t.Parallel()

	gw := New("test-key", 30*time.Second)
	_, err := gw.Refund(context.Background(), gateway.RefundRequest{})
	assert.EqualError(t, err, "midtrans: not implemented")
}
