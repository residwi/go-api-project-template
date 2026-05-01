package cart_test

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

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	cartMocks "github.com/residwi/go-api-project-template/mocks/cart"
)

func setupCartMux(t *testing.T) (*http.ServeMux, *cartMocks.MockRepository, *cartMocks.MockProductLookup, *cartMocks.MockStockChecker) {
	repo := cartMocks.NewMockRepository(t)
	products := cartMocks.NewMockProductLookup(t)
	stock := cartMocks.NewMockStockChecker(t)
	svc := cart.NewService(repo, nil, products, stock, 50)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	cart.RegisterRoutes(authed, cart.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, products, stock
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
		mux, repo, _, _ := setupCartMux(t)

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

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _ := setupCartMux(t)

		userID := uuid.New()
		repo.EXPECT().GetCart(mock.Anything, userID).Return(nil, core.ErrNotFound)

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
		mux, _, products, _ := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		products.EXPECT().GetByID(mock.Anything, productID).Return(nil, core.ErrNotFound)

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
		mux, repo, _, _ := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, core.ErrNotFound)

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
		mux, repo, _, _ := setupCartMux(t)

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
		mux, repo, _, _ := setupCartMux(t)

		userID := uuid.New()
		productID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, core.ErrNotFound)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_Clear(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _ := setupCartMux(t)

		userID := uuid.New()
		cartID := uuid.New()

		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(cartID, nil)
		repo.EXPECT().Clear(mock.Anything, cartID).Return(nil)

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _ := setupCartMux(t)

		userID := uuid.New()
		repo.EXPECT().GetOrCreate(mock.Anything, userID).Return(uuid.Nil, errors.New("db down"))

		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = authRequest(r, userID)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
