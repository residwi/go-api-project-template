package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_ListAll(t *testing.T) {
	t.Parallel()

	t.Run("success with pagination", func(t *testing.T) {
		t.Parallel()

		mux, _, admin := setupMux(t)

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
		admin.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("query.AdminListParams")).Return(orders, 1, nil)

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

		mux, _, admin := setupMux(t)
		admin.EXPECT().
			ListAdmin(mock.Anything, mock.AnythingOfType("query.AdminListParams")).
			Return(nil, 0, errors.New("db error"))

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

		mux, _, admin := setupMux(t)

		orderID := uuid.New()
		now := time.Now()
		admin.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
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

		mux, _, _ := setupMux(t)

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

		mux, _, admin := setupMux(t)
		orderID := uuid.New()
		admin.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/"+orderID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
