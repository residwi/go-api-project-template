package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success with pagination", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		now := time.Now()
		orders := []domain.Order{
			{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Status:    domain.StatusPaid,
				Subtotal:  money.New(10000, "USD"),
				Total:     money.New(10000, "USD"),
				CreatedAt: now,
				UpdatedAt: now,
			},
		}
		service.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).Return(orders, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?page=1&page_size=10", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)
		service.EXPECT().
			ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).
			Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		orderID := uuid.New()
		now := time.Now()
		service.EXPECT().Get(mock.Anything, orderID).Return(&domain.Order{
			ID:        orderID,
			UserID:    uuid.New(),
			Status:    domain.StatusPaid,
			Subtotal:  money.New(5000, "USD"),
			Total:     money.New(5000, "USD"),
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/"+orderID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/bad-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)
		orderID := uuid.New()
		service.EXPECT().Get(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/"+orderID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_UpdateStatus(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		orderID := uuid.New()
		service.EXPECT().ChangeStatus(mock.Anything, orderID, domain.Status("processing")).Return(nil)

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

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/bad-uuid/status", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

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

		mux, _ := setupAdminMux(t)

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

		mux, service := setupAdminMux(t)

		orderID := uuid.New()
		service.EXPECT().ChangeStatus(mock.Anything, orderID, mock.Anything).Return(apperror.ErrNotFound)

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

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockOrderManager) {
	service := NewMockOrderManager(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	h := NewAdminHandler(service, validator.New())
	admin.HandleFunc("GET /orders", h.List)
	admin.HandleFunc("GET /orders/{id}", h.Get)
	admin.HandleFunc("PUT /orders/{id}/status", h.UpdateStatus)

	return mux, service
}
