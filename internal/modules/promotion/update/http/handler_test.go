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
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_Update_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupUpdateMux(t)

		id := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, id, mock.Anything).Return(&domain.Promotion{
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
}

func TestAdminHandler_Update_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupUpdateMux(t)

		id := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, id, mock.Anything).Return(nil, apperror.ErrNotFound)

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

		mux, _ := setupUpdateMux(t)

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

		mux, _ := setupUpdateMux(t)

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

		mux, _ := setupUpdateMux(t)

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

func setupUpdateMux(t *testing.T) (*http.ServeMux, *MockPromotionUpdater) {
	t.Helper()

	cmd := NewMockPromotionUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	New(cmd, v).RegisterHTTP(admin)

	return mux, cmd
}
