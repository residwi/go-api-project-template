package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/cart"
)

func TestToCartResponse_FlagsUnsellableLineAndExcludesItFromTotal(t *testing.T) {
	sellableID, unsellableID := uuid.New(), uuid.New()

	c := &cart.Cart{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ID:        uuid.New(),
				ProductID: sellableID,
				Quantity:  2,
				Product:   &cart.Product{Name: "Widget", Price: 1000, Currency: "USD", Stock: 5, Status: "published"},
			},
			{
				ID:        uuid.New(),
				ProductID: unsellableID,
				Quantity:  3,
				// Archived after being added to the cart -- Phase 2's decision: keep
				// the line visible instead of dropping it silently.
				Product: &cart.Product{Name: "Gone", Price: 900, Currency: "USD", Stock: 0, Status: "archived"},
			},
		},
	}

	out := toCartResponse(c)

	require.Len(t, out.Items, 2, "an unsellable line must still be returned, not hidden")
	assert.True(t, out.Items[0].Sellable, "a published product's line must be sellable")
	assert.False(t, out.Items[1].Sellable, "an archived product's line must be flagged unsellable")
	assert.Equal(t, int64(2000), out.Total,
		"the unsellable line's 900*3=2700 must be excluded; only the sellable 1000*2=2000 counts")
}

func TestToCartResponse_MissingProductIsUnsellable(t *testing.T) {
	// Service.GetCart substitutes &Product{Status: "unavailable"} when the
	// product record is gone entirely (never leaves Product nil). Confirm this
	// synthetic placeholder is also treated as unsellable and excluded.
	c := &cart.Cart{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product:   &cart.Product{Status: "unavailable"},
			},
		},
	}

	out := toCartResponse(c)

	require.Len(t, out.Items, 1)
	assert.False(t, out.Items[0].Sellable)
	assert.Equal(t, int64(0), out.Total)
}

// TestToCartResponse_OmitsUserID pins cartResponse's top-level wire shape.
// cart is the only one of 14 features whose response drops a field
// (UserID) without a test pinning the key set -- this closes that gap the
// same way wishlist's TestToItemResponse_OmitsInternalFields pins WishlistID.
func TestToCartResponse_OmitsUserID(t *testing.T) {
	userID := uuid.New()

	c := &cart.Cart{
		ID:     uuid.New(),
		UserID: userID, // internal -- the caller is always the authenticated user
		Items:  []cart.Item{},
	}

	out := toCartResponse(c)

	raw, err := json.Marshal(out)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "items", "total"}, keysOf(fields),
		"the response must expose exactly these fields")
	assert.NotContains(t, string(raw), userID.String(),
		"the caller is always the authenticated user; echoing user_id back adds nothing")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
