package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_Clear_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase, uc := setupClearMux(t)

		usecase.EXPECT().Clear(mock.Anything, uc.UserID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_Clear_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, usecase, uc := setupClearMux(t)

		usecase.EXPECT().Clear(mock.Anything, uc.UserID).Return(errors.New("db down"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/cart", nil)
		r = withAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_Clear(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/cart", nil)
		w := httptest.NewRecorder()

		h.Clear(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func setupClearMux(t *testing.T) (*http.ServeMux, *MockCartClearer, middleware.UserContext) {
	usecase := NewMockCartClearer(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("DELETE /cart", New(usecase).Clear)

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
