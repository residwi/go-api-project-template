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
// toApplyResponse -- the same construction path the handler uses -- so the key
// set below is the real wire contract rather than a hand-built struct literal.
//
// What this does and does not guarantee: the ElementsMatch below catches any
// field added to applyResponse without `,omitempty`. It does NOT catch one added
// *with* `,omitempty`, because toApplyResponse takes only a code and a discount
// -- any new field is never assigned, so it is always zero and always omitted.
// The backstop for that case is the compiler: widening the response means
// widening this mapper's signature, which breaks every call site. Said plainly
// because a test comment that overclaims is worse than no comment -- it stops
// the next person looking.
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
