package http

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_ListUsers(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		now := time.Now()
		repo.EXPECT().List(mock.Anything, mock.Anything).Return([]user.User{
			{
				ID:        uuid.New(),
				Email:     "alice@example.com",
				FirstName: "Alice",
				LastName:  "Smith",
				Role:      "user",
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		items, ok := data["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 1)

		item := items[0].(map[string]any)
		itemJSON, err := json.Marshal(item)
		require.NoError(t, err)
		var gotItem listedUserItem
		require.NoError(t, json.Unmarshal(itemJSON, &gotItem))
		assert.Equal(t, listedUserItem{
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}, gotItem)
		assert.NotEmpty(t, item["id"])

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(1), pagination["current_page"], 0.0001)
		assert.InDelta(t, float64(20), pagination["page_size"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_items"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_pages"], 0.0001)
		assert.Equal(t, false, pagination["has_previous"])
		assert.Equal(t, false, pagination["has_next"])
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)
		repo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_GetUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		userID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(&user.User{
			ID:        userID,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Email string `json:"email"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Email string `json:"email"`
		}{Email: "alice@example.com"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupUserMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)
		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_UpdateUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		userID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(&user.User{
			ID:        userID,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

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

		mux, repo := setupUserMux(t)

		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

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

		mux, _ := setupUserMux(t)

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

func TestAdminHandler_UpdateRole(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()
		now := time.Now()

		repo.EXPECT().GetByID(mock.Anything, targetID).Return(&user.User{
			ID:        targetID,
			Email:     "target@example.com",
			FirstName: "Target",
			LastName:  "User",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

		sameID := uuid.New()
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

		mux, _ := setupUserMux(t)

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

func TestAdminHandler_DeleteUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()
		now := time.Now()

		repo.EXPECT().GetByID(mock.Anything, targetID).Return(&user.User{
			ID:        targetID,
			Email:     "target@example.com",
			FirstName: "Target",
			LastName:  "User",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

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

		mux, _ := setupUserMux(t)

		sameID := uuid.New()

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

func TestAdminHandler_ListUsers_WithActiveFilter(t *testing.T) {
	t.Parallel()

	t.Run("success with active filter", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		now := time.Now()
		repo.EXPECT().List(mock.Anything, mock.MatchedBy(func(p user.ListParams) bool {
			return p.Active != nil && *p.Active == true
		})).Return([]user.User{
			{
				ID:        uuid.New(),
				Email:     "active@example.com",
				FirstName: "Active",
				LastName:  "User",
				Role:      "user",
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?active=true", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		items, ok := data["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 1)

		item := items[0].(map[string]any)
		itemJSON, err := json.Marshal(item)
		require.NoError(t, err)
		var gotItem listedUserItem
		require.NoError(t, json.Unmarshal(itemJSON, &gotItem))
		assert.Equal(t, listedUserItem{
			Email:     "active@example.com",
			FirstName: "Active",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}, gotItem)
		assert.NotEmpty(t, item["id"])

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(1), pagination["current_page"], 0.0001)
		assert.InDelta(t, float64(20), pagination["page_size"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_items"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_pages"], 0.0001)
		assert.Equal(t, false, pagination["has_previous"])
		assert.Equal(t, false, pagination["has_next"])
	})

	t.Run("success with role filter", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupUserMux(t)

		now := time.Now()
		repo.EXPECT().List(mock.Anything, mock.MatchedBy(func(p user.ListParams) bool {
			return p.Role == "admin"
		})).Return([]user.User{
			{
				ID:        uuid.New(),
				Email:     "admin@example.com",
				FirstName: "Admin",
				LastName:  "User",
				Role:      "admin",
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?role=admin", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Items []struct {
				Email     string `json:"email"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
				Role      string `json:"role"`
				Active    bool   `json:"active"`
			} `json:"items"`
			Pagination struct {
				CurrentPage int  `json:"current_page"`
				PageSize    int  `json:"page_size"`
				TotalItems  int  `json:"total_items"`
				TotalPages  int  `json:"total_pages"`
				HasPrevious bool `json:"has_previous"`
				HasNext     bool `json:"has_next"`
			} `json:"pagination"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Len(t, got.Items, 1)
		assert.Equal(t, "admin@example.com", got.Items[0].Email)
		assert.Equal(t, "Admin", got.Items[0].FirstName)
		assert.Equal(t, "User", got.Items[0].LastName)
		assert.Equal(t, "admin", got.Items[0].Role)
		assert.True(t, got.Items[0].Active)
		assert.Equal(t, 1, got.Pagination.CurrentPage)
		assert.Equal(t, 20, got.Pagination.PageSize)
		assert.Equal(t, 1, got.Pagination.TotalItems)
		assert.Equal(t, 1, got.Pagination.TotalPages)
		assert.False(t, got.Pagination.HasPrevious)
		assert.False(t, got.Pagination.HasNext)
	})
}

// Like toUserResponse, toAdminUserResponse maps from user.User (model.go),
// which carries no json:"-" tags -- this explicit field list is the only
// thing keeping PasswordHash, TokenVersion, and DeletedAt off the wire, even
// though this response deliberately exposes role, active, and the
// timestamps. No other test in this file decodes created_at, updated_at, or
// the full item shape, so this is also the only assertion pinning the
// complete admin field set.
func TestToAdminUserResponse_ExposesOperatorFieldsButNotCredentials(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deletedAt := now.Add(time.Hour)

	got := toAdminUserResponse(&user.User{
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

type listedUserItem struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
}
