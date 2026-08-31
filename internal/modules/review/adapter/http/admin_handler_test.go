package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/response"
	"github.com/residwi/go-api-project-template/internal/platform/web"
)

func TestAdminHandler_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		reviewID := uuid.New()
		service.EXPECT().Delete(mock.Anything, reviewID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/"+reviewID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/bad", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		reviewID := uuid.New()
		service.EXPECT().Delete(mock.Anything, reviewID).Return(errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/reviews/"+reviewID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockReviewDeleter) {
	t.Helper()

	service := NewMockReviewDeleter(t)

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/v1/admin")
	admin.HandleFunc("DELETE /reviews/{id}", NewAdminHandler(service).Delete)

	return mux, service
}
