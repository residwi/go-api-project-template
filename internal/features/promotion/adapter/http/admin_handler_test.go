package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/features/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestAdminHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		service.EXPECT().
			Create(mock.Anything, "NEW10", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Promotion{
				Code: "NEW10",
			}, nil)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(map[string]any{
			"code":       "NEW10",
			"type":       domain.TypePercentage,
			"value":      10,
			"starts_at":  startsAt,
			"expires_at": expiresAt,
			"active":     true,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		service.EXPECT().
			Create(mock.Anything, "DUP", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errs.ErrConflict)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(map[string]any{
			"code":       "DUP",
			"type":       domain.TypePercentage,
			"value":      10,
			"starts_at":  startsAt,
			"expires_at": expiresAt,
			"active":     true,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestAdminHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		promos := []domain.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		service.EXPECT().
			ListAdmin(mock.Anything, promotion.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
			Return(promos, 2, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

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
			ListAdmin(mock.Anything, promotion.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
			Return(nil, 0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		id := uuid.New()
		service.EXPECT().
			Update(mock.Anything, id, "UPDATED", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Promotion{
				ID:        id,
				Code:      "UPDATED",
				Type:      domain.TypeFixedAmount,
				Value:     500,
				Active:    true,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		body, _ := json.Marshal(map[string]string{"code": "UPDATED"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		id := uuid.New()
		service.EXPECT().
			Update(mock.Anything, id, "UPDATED", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errs.ErrNotFound)

		body, _ := json.Marshal(map[string]string{"code": "UPDATED"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("validation error invalid type value", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		id := uuid.NewString()
		body, _ := json.Marshal(map[string]string{"type": "invalid_type"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("invalid JSON via mux", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		id := uuid.NewString()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id, strings.NewReader("{bad"))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid UUID via mux", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		body, _ := json.Marshal(map[string]string{"code": "test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/not-a-uuid", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})
}

func TestAdminHandler_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		id := uuid.New()
		service.EXPECT().Delete(mock.Anything, id).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		id := uuid.New()
		service.EXPECT().Delete(mock.Anything, id).Return(errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockPromotionManager) {
	t.Helper()

	service := NewMockPromotionManager(t)

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/v1/admin")

	ah := NewAdminHandler(service)
	admin.HandleFunc("GET /promotions", ah.List)
	admin.HandleFunc("POST /promotions", ah.Create)
	admin.HandleFunc("PUT /promotions/{id}", ah.Update)
	admin.HandleFunc("DELETE /promotions/{id}", ah.Delete)

	return mux, service
}
