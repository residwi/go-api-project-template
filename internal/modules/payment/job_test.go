package payment

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundJobPayload(t *testing.T) {
	t.Parallel()

	t.Run("marshals only the payload fields", func(t *testing.T) {
		t.Parallel()

		paymentID := uuid.New()
		orderID := uuid.New()

		out, err := json.Marshal(RefundJob{PaymentID: paymentID, OrderID: orderID, svc: &Service{}})

		require.NoError(t, err)

		var keys map[string]any
		require.NoError(t, json.Unmarshal(out, &keys))
		assert.ElementsMatch(t, []string{"PaymentID", "OrderID"}, slices.Collect(maps.Keys(keys)))
	})

	t.Run("kind names the payment queue", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "payment.refund", RefundJob{}.Kind())
	})
}
