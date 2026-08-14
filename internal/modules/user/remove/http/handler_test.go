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

		mux, cmd := setupDeleteMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		cmd.EXPECT().Execute(mock.Anything, mock.Anything).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+targetID.String(), nil)
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

		mux, _ := setupDeleteMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupDeleteMux(t)

		targetID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+targetID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "authentication required", resp.Error.Message)
	})

	t.Run("self-deletion blocked", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupDeleteMux(t)

		sameID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, mock.Anything).Return(apperror.ErrForbidden)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+sameID.String(), nil)
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
}

func setupDeleteMux(t *testing.T) (*http.ServeMux, *MockUserDeleter) {
	cmd := NewMockUserDeleter(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	admin.HandleFunc("DELETE /users/{id}", New(cmd).Delete)

	return mux, cmd
}
