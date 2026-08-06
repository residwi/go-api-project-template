// The TestCartHandler_ prefix is deliberate, not drift from the TestHandler_
// every other feature uses: this file's route-level and direct-call tests cover
// the same five methods, and identical names would compile but print
// indistinguishable `=== RUN` lines. Rename only if the direct-call tests go.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestCartHandler_GetCart(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		cartID := uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&cart.Cart{
			ID:     cartID,
			UserID: userID,
			Items:  []cart.Item{},
		}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	// An empty cart has no currency to denominate its sum in. The missing currency
	// must not read as a mismatch and turn `total: 0` into a 400.
	t.Run("empty cart returns total 0 with 200", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&cart.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []cart.Item{},
		}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(0), data["total"], 0.0001)
	})

	// A cart can hold lines in different currencies -- AddItem does not constrain
	// them -- and GET /cart answers 400, matching what PlaceOrder already does.
	//
	// Asserted at the mux: money.ErrCurrencyMismatch is not a case in
	// response.HandleErr, so alone it would be a 500. The wrapped
	// apperror.ErrBadRequest is what makes the 400.
	t.Run("mixed-currency cart returns 400", func(t *testing.T) {
		t.Parallel()

		mux, repo, products := setupCartMux(t)

		userID := uuid.New()
		usdID, eurID := uuid.New(), uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&cart.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items: []cart.Item{
				{ID: uuid.New(), ProductID: usdID, Quantity: 1},
				{ID: uuid.New(), ProductID: eurID, Quantity: 1},
			},
		}, nil)
		products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{usdID, eurID}).
			Return(map[uuid.UUID]productcontract.Product{
				usdID: {
					ID:        usdID,
					Name:      "Dollar Widget",
					Price:     money.New(1000, "USD"),
					Status:    "published",
					Available: 5,
				},
				eurID: {
					ID:        eurID,
					Name:      "Euro Widget",
					Price:     money.New(1000, "EUR"),
					Status:    "published",
					Available: 5,
				},
			}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusBadRequest, w.Code,
			"a cart whose lines cannot be summed must not answer 200 with a meaningless number")
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error.Message, "mixed currencies")
	})

	// An unsellable line in another currency must NOT trip the mismatch: it never
	// contributed to the total. Fold every line instead and this 200 becomes a 400.
	t.Run("unsellable line in another currency does not break the total", func(t *testing.T) {
		t.Parallel()

		mux, repo, products := setupCartMux(t)

		userID := uuid.New()
		liveID, goneID := uuid.New(), uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(&cart.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items: []cart.Item{
				{ID: uuid.New(), ProductID: liveID, Quantity: 2},
				{ID: uuid.New(), ProductID: goneID, Quantity: 3},
			},
		}, nil)
		products.EXPECT().GetInfoByIDs(mock.Anything, []uuid.UUID{liveID, goneID}).
			Return(map[uuid.UUID]productcontract.Product{
				liveID: {ID: liveID, Name: "Widget", Price: money.New(1000, "USD"), Status: "published", Available: 5},
				goneID: {ID: goneID, Name: "Archived", Price: money.New(900, "EUR"), Status: "archived", Available: 0},
			}, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(2000), data["total"], 0.0001,
			"only the sellable USD line counts: 1000*2, with the archived EUR line excluded")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestCartHandler_AddItem(t *testing.T) {
	t.Parallel()

	t.Run("service error product not found", func(t *testing.T) {
		t.Parallel()

		mux, _, products := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetInfo(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

		body := fmt.Sprintf(`{"product_id":"%s","quantity":1}`, productID)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(body))
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_UpdateItem(t *testing.T) {
	t.Parallel()

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo, products := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		// UpdateQuantity validates the product first; let that pass so GetOrCreate is
		// what fails here.
		products.EXPECT().GetInfo(mock.Anything, productID).
			Return(&productcontract.Product{ID: productID, Status: "published", Available: 10}, nil)
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, apperror.ErrNotFound)

		body := `{"quantity":3}`
		r := httptest.NewRequest(http.MethodPut, "/api/v1/cart/items/"+productID.String(), strings.NewReader(body))
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_RemoveItem(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().RemoveItem(mock.Anything, cartID, productID).Return(nil)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, apperror.ErrNotFound)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_Clear(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()

		repo.EXPECT().Clear(mock.Anything, userID).Return(nil)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo, _ := setupCartMux(t)

		userID := uuid.New()
		repo.EXPECT().Clear(mock.Anything, userID).Return(errors.New("db down"))

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetCart(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

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
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/cart/items", nil)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.AddItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader(`{}`))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.UpdateItem(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.RemoveItem(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Clear(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart", nil)
		w := httptest.NewRecorder()

		h.Clear(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestToCartResponse_FlagsUnsellableLineAndExcludesItFromTotal(t *testing.T) {
	t.Parallel()

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
				Product:   &cart.Product{Name: "Gone", Price: money.New(900, "USD"), Stock: 0, Status: "archived"},
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
	t.Parallel()

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

// Pins both sentinels, which the mux-level test cannot see: ErrBadRequest makes
// the 400, ErrCurrencyMismatch names the cause. Also pins that no response is
// built, since a zero total would be indistinguishable from an empty cart.
func TestToCartResponse_MixedCurrenciesRefusesToTotal(t *testing.T) {
	t.Parallel()

	c := &cart.Cart{
		ID: uuid.New(),
		Items: []cart.Item{
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product: &cart.Product{
					Name:   "Dollar Widget",
					Price:  money.New(1000, "USD"),
					Stock:  5,
					Status: "published",
				},
			},
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product: &cart.Product{
					Name:   "Euro Widget",
					Price:  money.New(1000, "EUR"),
					Stock:  5,
					Status: "published",
				},
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

func TestToCartResponse_OmitsUserID(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	c := &cart.Cart{
		ID:     uuid.New(),
		UserID: userID,
		Items:  []cart.Item{},
	}

	out, err := toCartResponse(c)
	require.NoError(t, err)

	raw, err := json.Marshal(out)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "items", "total"}, slices.Collect(maps.Keys(fields)),
		"the response must expose exactly these fields")
	assert.NotContains(t, string(raw), userID.String(),
		"the caller is always the authenticated user; echoing user_id back adds nothing")
}

type stubRepo struct {
	getOrCreateID uuid.UUID
}

func (s *stubRepo) GetOrCreate(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return s.getOrCreateID, nil
}

func (s *stubRepo) GetCart(
	context.Context,
	uuid.UUID,
) (*cart.Cart, error) {
	return nil, nil //nolint:nilnil // test stub
}

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

func (s *stubProducts) GetInfo(_ context.Context, id uuid.UUID) (*productcontract.Product, error) {
	return &productcontract.Product{
		ID:        id,
		Name:      "Widget",
		Price:     money.New(1000, "USD"),
		Status:    "published",
		Available: 10,
	}, nil
}

func (s *stubProducts) GetInfoByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error) {
	out := make(map[uuid.UUID]productcontract.Product, len(ids))
	for _, id := range ids {
		out[id] = productcontract.Product{
			ID:        id,
			Name:      "Widget",
			Price:     money.New(1000, "USD"),
			Status:    "published",
			Available: 10,
		}
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

func setupCartMux(t *testing.T) (*http.ServeMux, *MockRepository, *MockProductLookup) {
	repo := NewMockRepository(t)
	products := NewMockProductLookup(t)
	svc := cart.NewService(repo, testhelper.FakeTxRunner{}, products, 50)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	RegisterRoutes(authed, RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, products
}

func authRequest(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
