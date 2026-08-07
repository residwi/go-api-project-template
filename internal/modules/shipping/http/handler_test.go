package http

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/shipping"
)

func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

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
		slices.Collect(maps.Keys(fields)),
		"shipped_at and delivered_at are omitempty and absent when nil")
}
