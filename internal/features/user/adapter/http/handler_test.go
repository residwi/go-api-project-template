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

	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_Me(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupHandlerMux(t)

		userID := uuid.New()
		usecase.EXPECT().GetUser(mock.Anything, userID).Return(&domain.User{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Phone:     "+15551234567",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID,
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
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
		}{Email: "test@example.com", FirstName: "John"}, got)

		assertPublicProfileKeys(t, dataJSON)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupHandlerMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupHandlerMux(t)
		userID := uuid.New()
		usecase.EXPECT().GetUser(mock.Anything, userID).Return(nil, errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID, Role: "user",
		})
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupHandlerMux(t)

		userID := uuid.New()
		now := time.Now()
		usecase.EXPECT().UpdateProfile(mock.Anything, userID, "Jane", "", (*string)(nil)).Return(&domain.User{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "Jane",
			LastName:  "Doe",
			Phone:     "+15551234567",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		body, _ := json.Marshal(map[string]any{"first_name": "Jane"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID,
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

		assertPublicProfileKeys(t, dataJSON)
	})

	t.Run("validation error invalid first_name too long", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupHandlerMux(t)

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
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID,
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

		mux, _ := setupHandlerMux(t)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupHandlerMux(t)
		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID, Role: "user",
		})
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupHandlerMux(t)
		userID := uuid.New()
		usecase.EXPECT().UpdateProfile(mock.Anything, userID, "Jane", "", (*string)(nil)).
			Return(nil, errs.ErrNotFound)

		body, _ := json.Marshal(map[string]any{"first_name": "Jane"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{
			UserID: userID, Role: "user",
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

// assertPublicProfileKeys nets the swap TestToUserResponse cannot see.
// toUserResponse and toAdminUserResponse sit in one package, take the same
// argument and feed response.OK, which takes any -- so writing
// toAdminUserResponse at a /users/me call site compiles, and decoding the body
// into a narrow struct would not notice, because [json.Unmarshal] drops the extra
// keys silently. Every key of the raw body is checked instead.
func assertPublicProfileKeys(t *testing.T, dataJSON []byte) {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(dataJSON, &fields))
	assert.ElementsMatch(
		t,
		[]string{"id", "email", "first_name", "last_name", "phone"},
		slices.Collect(maps.Keys(fields)),
		"the authed profile endpoints serve the public mapper: role, active and the timestamps are admin-only",
	)
}

func setupHandlerMux(t *testing.T) (*http.ServeMux, *MockProfileManager) {
	usecase := NewMockProfileManager(t)

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	h := NewHandler(usecase)
	authed.HandleFunc("GET /users/me", h.Me)
	authed.HandleFunc("PUT /users/me", h.Update)

	return mux, usecase
}
