package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_Update_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase, uc := setupUpdateMux(t)

		productID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, uc.UserID, productID, mock.Anything).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/cart/items/"+productID.String(),
			strings.NewReader(`{"quantity":5}`),
		)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_Update_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("cart not found", func(t *testing.T) {
		t.Parallel()

		mux, usecase, uc := setupUpdateMux(t)

		productID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, uc.UserID, productID, mock.Anything).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/cart/items/"+productID.String(),
			strings.NewReader(`{"quantity":3}`),
		)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Parallel()

	h := &Handler{validator: validator.New()}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.Update(w, r)

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

		h.Update(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		productID := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/cart/items/"+productID, strings.NewReader("{bad"))
		r = setAuthContext(r)
		r.SetPathValue("product_id", productID)
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func setupUpdateMux(t *testing.T) (*http.ServeMux, *MockCartQuantityUpdater, middleware.UserContext) {
	usecase := NewMockCartQuantityUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("PUT /cart/items/{product_id}", New(usecase, v).Update)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, usecase, uc
}

func withAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
