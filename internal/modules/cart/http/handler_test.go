package http_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	carthttp "github.com/residwi/go-api-project-template/internal/modules/cart/http"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	cartMocks "github.com/residwi/go-api-project-template/mocks/cart"
)

func setupCartMux(t *testing.T) (*http.ServeMux, *cartMocks.MockRepository, *cartMocks.MockProductLookup) {
	repo := cartMocks.NewMockRepository(t)
	products := cartMocks.NewMockProductLookup(t)
	svc := cart.NewService(repo, testhelper.FakeTxRunner{}, products, 50)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	carthttp.RegisterRoutes(authed, carthttp.RouteDeps{
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

func TestCartHandler_GetCart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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

	// An empty cart has always answered `total: 0` with a 200, and must keep doing
	// so now that the total comes from Cart.Total(): with no sellable lines there
	// is no currency to denominate the sum in, so Total returns the zero Money.
	// Publishing its bare Amount gives the 0 clients already expect -- what must
	// not happen is the missing currency being treated as a mismatch and turning
	// an empty cart into a 400.
	t.Run("empty cart returns total 0 with 200", func(t *testing.T) {
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

	// THE ONE DELIBERATE BEHAVIOUR CHANGE IN THIS TASK.
	//
	// A cart can hold lines priced in different currencies: prices are
	// per-product, AddItem does not constrain them, and checkout is where the
	// combination is rejected. Before money.Money, GET /cart answered 200 with the
	// two amounts added together -- a number denominated in nothing, which is not a
	// total of anything. It is now a 400, matching what PlaceOrder already returns
	// for the same cart.
	//
	// Asserted at the mux, not with errors.Is, because the status code is what a
	// client observes: money.ErrCurrencyMismatch is not a case in
	// response.HandleErr, so surfacing it alone would be a 500. The wrapped
	// apperror.ErrBadRequest is what makes it a 400.
	t.Run("mixed-currency cart returns 400", func(t *testing.T) {
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
		products.EXPECT().GetByIDs(mock.Anything, []uuid.UUID{usdID, eurID}).
			Return(map[uuid.UUID]cart.ProductInfo{
				usdID: {ID: usdID, Name: "Dollar Widget", Price: money.New(1000, "USD"), Status: "published", Available: 5},
				eurID: {ID: eurID, Name: "Euro Widget", Price: money.New(1000, "EUR"), Status: "published", Available: 5},
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

	// The complement: one unsellable line in another currency must NOT trip the
	// mismatch, because it never contributed to the total in the first place. This
	// is the case that makes "sellable lines only" load-bearing rather than
	// incidental -- fold every line and this cart becomes a 400 that used to be a
	// perfectly good 200.
	t.Run("unsellable line in another currency does not break the total", func(t *testing.T) {
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
		products.EXPECT().GetByIDs(mock.Anything, []uuid.UUID{liveID, goneID}).
			Return(map[uuid.UUID]cart.ProductInfo{
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
	t.Run("service error product not found", func(t *testing.T) {
		mux, _, products := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

		body := fmt.Sprintf(`{"product_id":"%s","quantity":1}`, productID)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", strings.NewReader(body))
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_UpdateItem(t *testing.T) {
	t.Run("service error", func(t *testing.T) {
		mux, repo, products := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		// UpdateQuantity now validates the product (published + in stock) before
		// touching the cart; let that pass so GetOrCreate is what fails here.
		products.EXPECT().GetByID(mock.Anything, productID).
			Return(&cart.ProductInfo{ID: productID, Status: "published", Available: 10}, nil)
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
	t.Run("success", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
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
