package http

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyResponse_OmitsUsageCountersAndLimits pins the plan's callout: the
// public apply response returns only the computed discount, never the
// promotion's internal usage counters or per-user limits. There is no
// mapper here (applyResponse is built directly from the request code and
// the computed discount, never from a promotion.Promotion), so this test
// pins the type's field set directly -- a field added to applyResponse to
// echo back MinOrderAmount/MaxDiscount/MaxUses/UsedCount must fail this.
func TestApplyResponse_OmitsUsageCountersAndLimits(t *testing.T) {
	got := applyResponse{Code: "SAVE10", Discount: 424242}

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
