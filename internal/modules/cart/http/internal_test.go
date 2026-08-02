package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

type stubRepo struct {
	getOrCreateID uuid.UUID
}

func (s *stubRepo) GetOrCreate(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return s.getOrCreateID, nil
}
func (s *stubRepo) GetCart(context.Context, uuid.UUID) (*cart.Cart, error) { return nil, nil } //nolint:nilnil // test stub
func (s *stubRepo) AddItem(_ context.Context, _, _ uuid.UUID, _ int) error {
	return nil
}

func (s *stubRepo) UpdateItemQuantity(_ context.Context, _, _ uuid.UUID, _ int) error {
	return nil
}
func (s *stubRepo) RemoveItem(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *stubRepo) Clear(context.Context, uuid.UUID) error                 { return nil }
func (s *stubRepo) CountItems(context.Context, uuid.UUID) (int, error)     { return 0, nil }
func (s *stubRepo) CountAndHasItem(context.Context, uuid.UUID, uuid.UUID) (int, bool, error) {
	return 0, false, nil
}

func (s *stubRepo) GetCartForLock(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

type stubProducts struct{}

func (s *stubProducts) GetByID(_ context.Context, id uuid.UUID) (*cart.ProductInfo, error) {
	return &cart.ProductInfo{ID: id, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}, nil
}

func (s *stubProducts) GetByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]cart.ProductInfo, error) {
	out := make(map[uuid.UUID]cart.ProductInfo, len(ids))
	for _, id := range ids {
		out[id] = cart.ProductInfo{ID: id, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 10}
	}
	return out, nil
}

func newTestHandler() *handler {
	return &handler{
		service:   &cart.Service{},
		validator: validator.New(),
	}
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

func TestHandler_GetCart(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/cart", nil)
		w := httptest.NewRecorder()

		h.GetCart(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})
}

func TestHandler_AddItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", nil)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})

	t.Run("success", func(t *testing.T) {
		repo := &stubRepo{getOrCreateID: uuid.New()}
		svc := cart.NewService(repo, testhelper.FakeTxRunner{}, &stubProducts{}, 50)
		h := &handler{service: svc, validator: validator.New()}

		userID := uuid.New()
		productID := uuid.New()

		body := fmt.Sprintf(`{"product_id":"%s","quantity":2}`, productID)
		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(body))
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestHandler_UpdateItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid product_id")
	})

	t.Run("validation error missing quantity", func(t *testing.T) {
		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader(`{}`))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		h := newTestHandler()
		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader("{bad"))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		repo := &stubRepo{getOrCreateID: uuid.New()}
		svc := cart.NewService(repo, testhelper.FakeTxRunner{}, &stubProducts{}, 50)
		h := &handler{service: svc, validator: validator.New()}

		userID := uuid.New()
		productID := uuid.New()

		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID.String(), strings.NewReader(`{"quantity":5}`))
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		r.SetPathValue("product_id", productID.String())
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_RemoveItem(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Clear(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/cart", nil)
		w := httptest.NewRecorder()

		h.Clear(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

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
// failure carries, which the mux-level test in handler_test.go
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
