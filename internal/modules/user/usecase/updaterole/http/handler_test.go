package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupUpdateRoleMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(map[string]any{"role": "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: requesterID,
			Email:  "admin@example.com",
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupUpdateRoleMux(t)

		body, _ := json.Marshal(map[string]any{"role": "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/not-a-uuid/role", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupUpdateRoleMux(t)

		targetID := uuid.New()
		body, _ := json.Marshal(map[string]any{"role": "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "authentication required", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupUpdateRoleMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader([]byte("{invalid")),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: requesterID,
			Email:  "admin@example.com",
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("self-demotion blocked", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupUpdateRoleMux(t)

		sameID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, mock.Anything).
			Return(apperror.ErrForbidden)
		body, _ := json.Marshal(map[string]any{"role": "user"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+sameID.String()+"/role", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: sameID,
			Email:  "admin@example.com",
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("validation error invalid role", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupUpdateRoleMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()
		body, _ := json.Marshal(map[string]any{"role": "superadmin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: requesterID,
			Email:  "admin@example.com",
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func setupUpdateRoleMux(t *testing.T) (*http.ServeMux, *MockRoleUpdater) {
	cmd := NewMockRoleUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	admin.HandleFunc("PUT /users/{id}/role", New(cmd, v).Update)

	return mux, cmd
}
