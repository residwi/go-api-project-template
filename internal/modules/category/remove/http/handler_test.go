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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupRemoveMux(t)

		catID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, catID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupRemoveMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("command error", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupRemoveMux(t)

		catID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, catID).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupRemoveMux(t *testing.T) (*http.ServeMux, *MockCategoryDeleter) {
	t.Helper()

	cmd := NewMockCategoryDeleter(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	admin.HandleFunc("DELETE /categories/{id}", New(cmd).Delete)

	return mux, cmd
}
