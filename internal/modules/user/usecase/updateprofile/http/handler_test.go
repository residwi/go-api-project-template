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

		mux, usecase := setupProfileMux(t)

		userID := uuid.New()
		now := time.Now()
		usecase.EXPECT().Execute(mock.Anything, userID, mock.Anything).Return(&domain.User{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "Jane",
			LastName:  "Doe",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		body, _ := json.Marshal(map[string]any{"first_name": "Jane"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			FirstName string `json:"first_name"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			FirstName string `json:"first_name"`
		}{FirstName: "Jane"}, got)
	})

	t.Run("validation error invalid first_name too long", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProfileMux(t)

		userID := uuid.New()
		longName := make([]byte, 101)
		for i := range longName {
			longName[i] = 'a'
		}

		body, _ := json.Marshal(map[string]string{
			"first_name": string(longName),
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProfileMux(t)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProfileMux(t)
		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupProfileMux(t)
		userID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, userID, mock.Anything).Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{"first_name": "Jane"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestToUserResponse_OmitsCredentialAndAuthInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toUserResponse(&domain.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: "$2a$10$distinguishablebcryptvalue",
		FirstName:    "John",
		LastName:     "Doe",
		Phone:        "+15551234567",
		Role:         "admin",
		Active:       false,
		TokenVersion: 424242,
		CreatedAt:    deletedAt,
		UpdatedAt:    deletedAt,
		DeletedAt:    &deletedAt,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(
		t,
		[]string{"id", "email", "first_name", "last_name", "phone"},
		slices.Collect(maps.Keys(fields)),
		"the public profile must expose exactly these fields",
	)

	assert.NotContains(t, string(raw), "distinguishablebcryptvalue",
		"PasswordHash is credential material and must never be serialised")
	assert.NotContains(t, string(raw), "424242",
		"TokenVersion is auth-internal revocation state and must never be serialised")
	assert.NotContains(t, string(raw), `"role"`,
		"role is reserved for the admin response")
	assert.NotContains(t, string(raw), `"active"`,
		"active is reserved for the admin response")
	assert.NotContains(t, string(raw), "2026-01-01",
		"timestamps are reserved for the admin response")
}

func setupProfileMux(t *testing.T) (*http.ServeMux, *MockProfileUpdater) {
	usecase := NewMockProfileUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("PUT /users/me", New(usecase, v).Update)

	return mux, usecase
}
