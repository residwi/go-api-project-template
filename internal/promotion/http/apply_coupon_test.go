package http

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyResponse_OmitsUsageCountersAndLimits pins the plan's callout: the
// public apply response returns only the computed discount, never the
// promotion's internal usage counters or per-user limits. It goes through
// toApplyResponse -- the same construction path the handler uses -- with a
// fixture whose fields are all non-zero, so a field re-added to applyResponse
// with `,omitempty` can't slip past this by coincidentally being zero-valued.
func TestApplyResponse_OmitsUsageCountersAndLimits(t *testing.T) {
	got := toApplyResponse("SAVE10", 424242)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"code", "discount"}, keysOf(fields),
		"the apply response must expose exactly the code and the computed discount")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
