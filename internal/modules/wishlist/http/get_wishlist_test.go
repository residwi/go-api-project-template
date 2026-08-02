package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist"
)

func TestToItemResponse_OmitsInternalFields(t *testing.T) {
	itemID, productID, listID := uuid.New(), uuid.New(), uuid.New()
	created := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	got := toItemResponse(wishlist.Item{
		ID:         itemID,
		WishlistID: listID, // internal -- must not reach the wire
		ProductID:  productID,
		CreatedAt:  created,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	assert.ElementsMatch(t, []string{"id", "product_id", "created_at"}, keysOf(fields),
		"the response must expose exactly these fields")
	assert.NotContains(t, string(raw), listID.String(),
		"wishlist_id is an internal join key and must not be serialised")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
