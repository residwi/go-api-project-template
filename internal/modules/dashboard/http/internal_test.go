package http

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
)

// TestToSummaryResponse_ExposesExactFieldSet pins summaryResponse's wire
// shape. dashboard's domain types are already the reporting read-model --
// purpose-built for this admin UI, not reused from any other feature -- so
// nothing is omitted here; the mapping is still explicit and still tested
// so a field added to SalesSummary or StatusBreakdown later does not
// silently ride onto the wire without this test being updated.
func TestToSummaryResponse_ExposesExactFieldSet(t *testing.T) {
	got := toSummaryResponse(
		dashboard.SalesSummary{TotalOrders: 10, TotalRevenue: 50000, AverageOrderValue: 5000},
		[]dashboard.StatusBreakdown{{Status: "paid", Count: 7}},
	)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"sales", "status_breakdown"}, keysOf(fields))

	var sales map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["sales"], &sales))
	assert.ElementsMatch(t, []string{"total_orders", "total_revenue", "average_order_value"}, keysOf(sales))

	var breakdown []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["status_breakdown"], &breakdown))
	require.Len(t, breakdown, 1)
	assert.ElementsMatch(t, []string{"status", "count"}, keysOf(breakdown[0]))
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
