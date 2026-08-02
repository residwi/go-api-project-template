package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/money"
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
				Product:   &cart.Product{Name: "Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published"},
			},
			{
				ID:        uuid.New(),
				ProductID: unsellableID,
				Quantity:  3,
				// Archived after being added to the cart -- Phase 2's decision: keep
				// the line visible instead of dropping it silently.
				Product: &cart.Product{Name: "Gone", Price: money.New(900, "USD"), Stock: 0, Status: "archived"},
			},
		},
	}

	out, err := toCartResponse(c)
	require.NoError(t, err)

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

	out, err := toCartResponse(c)
	require.NoError(t, err)

	require.Len(t, out.Items, 1)
	assert.False(t, out.Items[0].Sellable)
	assert.Equal(t, int64(0), out.Total)
}

// TestToCartResponse_MixedCurrenciesRefusesToTotal pins the two sentinels the
// failure carries, which the mux-level test in handler_integration_test.go
// cannot see: apperror.ErrBadRequest is what makes the status a 400 rather than
// the 500 an unrecognised error would produce, and money.ErrCurrencyMismatch is
// what names the cause for a log. Dropping either leaves the other test passing
// for the wrong reason -- or not passing at all -- so both are asserted here.
//
// It also pins that no response is built on the way out: publishing the items
// with a zero total would be worse than the 400, since a client cannot tell an
// empty cart from one that could not be added up.
func TestToCartResponse_MixedCurrenciesRefusesToTotal(t *testing.T) {
	c := &cart.Cart{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product:   &cart.Product{Name: "Dollar Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published"},
			},
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product:   &cart.Product{Name: "Euro Widget", Price: money.New(1000, "EUR"), Stock: 5, Status: "published"},
			},
		},
	}

	out, err := toCartResponse(c)

	require.Error(t, err)
	require.ErrorIs(t, err, apperror.ErrBadRequest,
		"a cart's contents are user input, so an unsummable cart is a 400 and not a 500")
	require.ErrorIs(t, err, money.ErrCurrencyMismatch,
		"the cause must stay matchable, not be flattened into a generic bad request")
	assert.Equal(t, cartResponse{}, out,
		"no partial response: a total that could not be computed must not ship as one that could")
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

	out, err := toCartResponse(c)
	require.NoError(t, err)

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
