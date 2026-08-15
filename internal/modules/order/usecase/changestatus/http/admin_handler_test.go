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
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_UpdateStatus(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupMux(t)

		orderID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, orderID, domain.Status("processing")).Return(nil)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/orders/"+orderID.String()+"/status",
			strings.NewReader(body),
		)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/bad-uuid/status", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/orders/"+orderID.String()+"/status",
			strings.NewReader("{invalid"),
		)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing status", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/orders/"+orderID.String()+"/status",
			strings.NewReader(`{}`),
		)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupMux(t)

		orderID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, orderID, mock.Anything).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/orders/"+orderID.String()+"/status",
			strings.NewReader(body),
		)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupMux(t *testing.T) (*http.ServeMux, *MockStatusChanger) {
	usecase := NewMockStatusChanger(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	admin.HandleFunc("PUT /orders/{id}/status", NewAdmin(usecase, v).UpdateStatus)

	return mux, usecase
}
