package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/inventory"
)

// TestToStockResponse_ExposesExactFieldSet pins stockResponse's wire shape.
// Every inventory route is admin-only, so Reserved is deliberately present
// here -- the reservation-count leak this phase closes is on product's
// public response (see product/http/list_products_test.go), not this one.
func TestToStockResponse_ExposesExactFieldSet(t *testing.T) {
	got := toStockResponse(&inventory.Stock{
		ProductID: uuid.New(),
		Quantity:  100,
		Reserved:  30,
		Available: 70,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"product_id", "quantity", "reserved", "available"}, keysOf(fields))
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
