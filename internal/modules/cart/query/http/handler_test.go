package http

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupGetMux(t)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupGetMux(t)

		userID := uuid.New()
		cartID := uuid.New()
		reader.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			ID:     cartID,
			UserID: userID,
			Items:  []domain.Item{},
		}, nil)

		r := authGet(userID)
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

		mux, reader := setupGetMux(t)

		userID := uuid.New()
		reader.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items:  []domain.Item{},
		}, nil)

		r := authGet(userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(0), data["total"], 0.0001)
	})

	// A cart can hold lines in different currencies -- add/ does not constrain
	// them -- and GET /cart answers 400, matching what PlaceOrder already does.
	//
	// Asserted at the mux: money.ErrCurrencyMismatch is not a case in
	// response.HandleErr, so alone it would be a 500. The wrapped
	// apperror.ErrBadRequest is what makes the 400.
	t.Run("mixed-currency cart returns 400", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupGetMux(t)

		userID := uuid.New()
		usdID, eurID := uuid.New(), uuid.New()
		reader.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items: []domain.Item{
				{ID: uuid.New(), ProductID: usdID, Quantity: 1, Product: &domain.Product{
					Name: "Dollar Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published",
				}},
				{ID: uuid.New(), ProductID: eurID, Quantity: 1, Product: &domain.Product{
					Name: "Euro Widget", Price: money.New(1000, "EUR"), Stock: 5, Status: "published",
				}},
			},
		}, nil)

		r := authGet(userID)
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

		mux, reader := setupGetMux(t)

		userID := uuid.New()
		liveID, goneID := uuid.New(), uuid.New()
		reader.EXPECT().GetCart(mock.Anything, userID).Return(&domain.Cart{
			ID:     uuid.New(),
			UserID: userID,
			Items: []domain.Item{
				{ID: uuid.New(), ProductID: liveID, Quantity: 2, Product: &domain.Product{
					Name: "Widget", Price: money.New(1000, "USD"), Stock: 5, Status: "published",
				}},
				{ID: uuid.New(), ProductID: goneID, Quantity: 3, Product: &domain.Product{
					Name: "Archived", Price: money.New(900, "EUR"), Stock: 0, Status: "archived",
				}},
			},
		}, nil)

		r := authGet(userID)
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

		mux, reader := setupGetMux(t)

		userID := uuid.New()
		reader.EXPECT().GetCart(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		r := authGet(userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
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

func setupGetMux(t *testing.T) (*http.ServeMux, *MockCartReader) {
	reader := NewMockCartReader(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	New(reader).RegisterHTTP(authed)

	return mux, reader
}

func authGet(userID uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
