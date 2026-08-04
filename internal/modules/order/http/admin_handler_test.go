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
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_ListAll(t *testing.T) {
	t.Parallel()

	t.Run("success with pagination", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupOrderMux(t)

		now := time.Now()
		orders := []order.Order{
			{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Status:    order.StatusPaid,
				Subtotal:  money.New(10000, "USD"),
				Total:     money.New(10000, "USD"),
				CreatedAt: now,
				UpdatedAt: now,
			},
		}
		repo.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).Return(orders, 1, nil)

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

		mux, repo, _, _ := setupOrderMux(t)
		repo.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_GetOrder(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    uuid.New(),
			Status:    order.StatusPaid,
			Subtotal:  money.New(5000, "USD"),
			Total:     money.New(5000, "USD"),
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, repo, _, _ := setupOrderMux(t)
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

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

		mux, repo, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    uuid.New(),
			Status:    order.StatusPaid,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusPaid, order.Status("processing")).Return(nil)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/bad-uuid/status", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader("{invalid"))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing status", func(t *testing.T) {
		t.Parallel()

		mux, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(`{}`))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
