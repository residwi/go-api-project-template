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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_Create_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(map[string]any{
			"code":       "NEW10",
			"type":       promotion.TypePercentage,
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
}

func TestAdminHandler_Create_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("repo conflict", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(apperror.ErrConflict)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(map[string]any{
			"code":       "DUP",
			"type":       promotion.TypePercentage,
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
}

func TestAdminHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		promos := []promotion.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		repo.EXPECT().
			ListAdmin(mock.Anything, promotion.ListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
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

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().
			ListAdmin(mock.Anything, promotion.ListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
			Return(nil, 0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_Update_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&promotion.Promotion{
			ID:        id,
			Code:      "OLD",
			Type:      promotion.TypeFixedAmount,
			Value:     500,
			Active:    true,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

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
}

func TestAdminHandler_Update_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]string{"code": "UPDATED"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_Update_ValidationError(t *testing.T) {
	t.Parallel()

	t.Run("invalid type value", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupPromotionMux(t)

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
}

func TestAdminHandler_Update_InvalidJSON(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON via mux", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupPromotionMux(t)

		id := uuid.NewString()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id, strings.NewReader("{bad"))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAdminHandler_Update_InvalidUUID(t *testing.T) {
	t.Parallel()

	t.Run("invalid UUID via mux", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupPromotionMux(t)

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

func TestAdminHandler_Delete_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestAdminHandler_Delete_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_AdminCreate(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader("{bad"))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})
}

func TestHandler_AdminUpdate(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		id := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/promotions/"+id, strings.NewReader("{bad"))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_AdminDelete(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodDelete, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Delete(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})
}

func newTestAdminHandler() *adminHandler {
	return &adminHandler{
		service:   &promotion.Service{},
		validator: validator.New(),
	}
}
