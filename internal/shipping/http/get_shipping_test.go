package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/shipping"
)

// TestToShipmentResponse_ExposesExactFieldSet pins shipmentResponse's wire
// shape. Nothing on shipping.Shipment is internal or sensitive, so this
// mirrors the domain model 1:1 -- but the mapping is still explicit and
// still tested, so a field added to Shipment later does not silently ride
// onto the wire without this test being updated to acknowledge it.
func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toShipmentResponse(&shipping.Shipment{
		ID:             uuid.New(),
		OrderID:        uuid.New(),
		Carrier:        "FedEx",
		TrackingNumber: "TRACK123",
		Status:         shipping.StatusShipped,
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "order_id", "carrier", "tracking_number", "status", "created_at", "updated_at"},
		keysOf(fields),
		"shipped_at and delivered_at are omitempty and absent when nil")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
