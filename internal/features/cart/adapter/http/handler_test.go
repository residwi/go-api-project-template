package http

import (
	"encoding/json"
	"errors"
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

	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		cartID := uuid.New()
		service.EXPECT().Get(mock.Anything, uc.UserID).Return(&domain.Cart{
			ID:     cartID,
			UserID: uc.UserID,
			Items:  []domain.Item{},
		}, nil)

		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil), uc)
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

		mux, service, uc := setupMux(t)

		service.EXPECT().Get(mock.Anything, uc.UserID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: uc.UserID,
			Items:  []domain.Item{},
		}, nil)

		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(0), data["total"], 0.0001)
	})

	// A cart can hold lines in different currencies -- Add does not constrain
	// them -- and GET /cart answers 400, matching what PlaceOrder already does.
	//
	// Asserted at the mux: money.ErrCurrencyMismatch is not a case in
	// response.HandleErr, so alone it would be a 500. The wrapped
	// errs.ErrBadRequest is what makes the 400.
	t.Run("mixed-currency cart returns 400", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		usdID, eurID := uuid.New(), uuid.New()
		service.EXPECT().Get(mock.Anything, uc.UserID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: uc.UserID,
			Items: []domain.Item{
				{ID: uuid.New(), ProductID: usdID, Quantity: 1, Product: &domain.Product{
					Name: "Dollar Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published",
				}},
				{ID: uuid.New(), ProductID: eurID, Quantity: 1, Product: &domain.Product{
					Name: "Euro Widget", Price: money.New(1000, "EUR"), Stock: 5, Status: "published",
				}},
			},
		}, nil)

		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil), uc)
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

		mux, service, uc := setupMux(t)

		liveID, goneID := uuid.New(), uuid.New()
		service.EXPECT().Get(mock.Anything, uc.UserID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: uc.UserID,
			Items: []domain.Item{
				{ID: uuid.New(), ProductID: liveID, Quantity: 2, Product: &domain.Product{
					Name: "Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published",
				}},
				{ID: uuid.New(), ProductID: goneID, Quantity: 3, Product: &domain.Product{
					Name: "Archived", Price: money.New(900, "EUR"), Stock: 0, Status: "archived",
				}},
			},
		}, nil)

		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil), uc)
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

		mux, service, uc := setupMux(t)

		service.EXPECT().Get(mock.Anything, uc.UserID).Return(nil, errs.ErrNotFound)

		r := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestHandler_Add(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader("{bad")), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(`{}`)), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

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

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Add(mock.Anything, uc.UserID, productID, 2).Return(nil)

		body := `{"product_id":"` + productID.String() + `","quantity":2}`
		r := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(body)), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("product not found", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Add(mock.Anything, uc.UserID, productID, 1).Return(errs.ErrNotFound)

		body := `{"product_id":"` + productID.String() + `","quantity":1}`
		r := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(body)), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodPut, "/api/v1/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := withAuth(httptest.NewRequest(http.MethodPut, "/api/v1/cart/items/bad", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid product_id")
	})

	t.Run("validation error missing quantity", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		productID := uuid.NewString()
		r := withAuth(httptest.NewRequest(http.MethodPut, "/api/v1/cart/items/"+productID, strings.NewReader(`{}`)), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		productID := uuid.NewString()
		r := withAuth(
			httptest.NewRequest(http.MethodPut, "/api/v1/cart/items/"+productID, strings.NewReader("{bad")),
			uc,
		)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().UpdateQuantity(mock.Anything, uc.UserID, productID, 5).Return(nil)

		r := withAuth(httptest.NewRequest(
			http.MethodPut,
			"/api/v1/cart/items/"+productID.String(),
			strings.NewReader(`{"quantity":5}`),
		), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("cart not found", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().UpdateQuantity(mock.Anything, uc.UserID, productID, 3).Return(errs.ErrNotFound)

		r := withAuth(httptest.NewRequest(
			http.MethodPut,
			"/api/v1/cart/items/"+productID.String(),
			strings.NewReader(`{"quantity":3}`),
		), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Remove(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

		r := withAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/bad", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Remove(mock.Anything, uc.UserID, productID).Return(nil)

		r := withAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		productID := uuid.New()
		service.EXPECT().Remove(mock.Anything, uc.UserID, productID).Return(errs.ErrNotFound)

		r := withAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Clear(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _, _ := setupMux(t)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		service.EXPECT().Clear(mock.Anything, uc.UserID).Return(nil)

		r := withAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		service.EXPECT().Clear(mock.Anything, uc.UserID).Return(errors.New("db down"))

		r := withAuth(httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil), uc)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestToCartResponse_FlagsUnsellableLineAndExcludesItFromTotal(t *testing.T) {
	t.Parallel()

	sellableID, unsellableID := uuid.New(), uuid.New()

	c := &domain.Cart{
		ID: uuid.New(),
		Items: []domain.Item{
			{
				ID:        uuid.New(),
				ProductID: sellableID,
				Quantity:  2,
				Product: &domain.Product{
					Name:   "Widget",
					Price:  money.New(1000, "USD"),
					Stock:  5,
					Status: "published",
				},
			},
			{
				ID:        uuid.New(),
				ProductID: unsellableID,
				Quantity:  3,
				Product:   &domain.Product{Name: "Gone", Price: money.New(900, "USD"), Stock: 0, Status: "archived"},
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

	c := &domain.Cart{
		ID: uuid.New(),
		Items: []domain.Item{
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product:   &domain.Product{Status: "unavailable"},
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

	c := &domain.Cart{
		ID: uuid.New(),
		Items: []domain.Item{
			{
				ID:        uuid.New(),
				ProductID: uuid.New(),
				Quantity:  1,
				Product: &domain.Product{
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
				Product: &domain.Product{
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
	require.ErrorIs(t, err, errs.ErrBadRequest,
		"a cart's contents are user input, so an unsummable cart is a 400 and not a 500")
	require.ErrorIs(t, err, money.ErrCurrencyMismatch,
		"the cause must stay matchable, not be flattened into a generic bad request")
	assert.Equal(t, cartResponse{}, out,
		"no partial response: a total that could not be computed must not ship as one that could")
}

func TestToCartResponse_OmitsUserID(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	c := &domain.Cart{
		ID:     uuid.New(),
		UserID: userID,
		Items:  []domain.Item{},
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

func setupMux(t *testing.T) (*http.ServeMux, *MockCartManager, middleware.UserContext) {
	service := NewMockCartManager(t)

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	h := NewHandler(service)
	authed.HandleFunc("GET /cart", h.Get)
	authed.HandleFunc("DELETE /cart", h.Clear)
	authed.HandleFunc("POST /cart/items", h.Add)
	authed.HandleFunc("PUT /cart/items/{product_id}", h.Update)
	authed.HandleFunc("DELETE /cart/items/{product_id}", h.Remove)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, service, uc
}

func withAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}
