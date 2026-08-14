package http

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupAdminUpdateMux(t)

		userID := uuid.New()
		now := time.Now()
		cmd.EXPECT().Execute(mock.Anything, userID, mock.Anything).Return(&domain.User{
			ID:        userID,
			Email:     "alice@example.com",
			FirstName: "Updated",
			LastName:  "Name",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		body, _ := json.Marshal(map[string]any{
			"first_name": "Updated",
			"last_name":  "Name",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+userID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		}{FirstName: "Updated", LastName: "Name"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminUpdateMux(t)

		body, _ := json.Marshal(map[string]any{"first_name": "Test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/not-a-uuid", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminUpdateMux(t)

		userID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+userID.String(),
			bytes.NewReader([]byte("{invalid")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupAdminUpdateMux(t)

		userID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, userID, mock.Anything).Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{"first_name": "Test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+userID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("validation error first_name too long", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminUpdateMux(t)

		userID := uuid.New()
		longName := make([]byte, 101)
		for i := range longName {
			longName[i] = 'a'
		}

		body, _ := json.Marshal(map[string]string{
			"first_name": string(longName),
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+userID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestToAdminUserResponse_ExposesOperatorFieldsButNotCredentials(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deletedAt := now.Add(time.Hour)

	got := toAdminUserResponse(&domain.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: "$2a$10$distinguishablebcryptvalue",
		FirstName:    "John",
		LastName:     "Doe",
		Phone:        "+15551234567",
		Role:         "admin",
		Active:       true,
		TokenVersion: 424242,
		CreatedAt:    now,
		UpdatedAt:    now,
		DeletedAt:    &deletedAt,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "email", "first_name", "last_name", "phone", "role", "active", "created_at", "updated_at"},
		slices.Collect(maps.Keys(fields)),
		"the admin response must expose exactly these fields")

	assert.NotContains(t, string(raw), "distinguishablebcryptvalue",
		"PasswordHash is credential material and must never be serialised, even to an admin")
	assert.NotContains(t, string(raw), "424242",
		"TokenVersion is auth-internal revocation state and must never be serialised")
}

func setupAdminUpdateMux(t *testing.T) (*http.ServeMux, *MockUserUpdater) {
	cmd := NewMockUserUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	admin.HandleFunc("PUT /users/{id}", New(cmd, v).Update)

	return mux, cmd
}
