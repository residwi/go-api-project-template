package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_Remove_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd, uc := setupRemoveMux(t)

		productID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, uc.UserID, productID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_Remove_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, cmd, uc := setupRemoveMux(t)

		productID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, uc.UserID, productID).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart/items/"+productID.String(), nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Remove(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart/items/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()

		h.Remove(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid product UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart/items/bad", nil)
		r = setAuthContext(r)
		r.SetPathValue("product_id", "bad")
		w := httptest.NewRecorder()

		h.Remove(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func setupRemoveMux(t *testing.T) (*http.ServeMux, *MockCartItemRemover, middleware.UserContext) {
	cmd := NewMockCartItemRemover(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("DELETE /cart/items/{product_id}", New(cmd).Remove)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, cmd, uc
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
