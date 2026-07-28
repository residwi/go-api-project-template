package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/payment"
)

// TestToAdminPaymentResponse_OmitsGatewayResponse pins the plan's
// highest-value payment control: GatewayResponse carries raw gateway
// payloads that may include PII or card metadata, and must never reach the
// wire -- not even to an admin. Every other Payment field, including
// OrderID, is operator-facing and stays.
func TestToAdminPaymentResponse_OmitsGatewayResponse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toAdminPaymentResponse(&payment.Payment{
		ID:              uuid.New(),
		OrderID:         uuid.New(),
		Amount:          5000,
		Currency:        "USD",
		Status:          payment.StatusSuccess,
		Method:          "card",
		PaymentMethodID: "pm_test_123",
		GatewayTxnID:    "txn_123",
		GatewayResponse: []byte(`{"card_number":"4242424242424242","cvv":"123"}`), // must not reach the wire
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "order_id", "amount", "currency", "status", "method", "payment_method_id",
			"gateway_txn_id", "created_at", "updated_at",
		},
		keysOf(fields),
		"payment_url and paid_at are omitempty and absent when unset; every other field must be present")

	assert.NotContains(t, string(raw), "4242424242424242",
		"GatewayResponse may carry PII or card metadata and must never be serialised, even to an admin")
	assert.NotContains(t, string(raw), "gateway_response",
		"the GatewayResponse field must not appear under any key")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
