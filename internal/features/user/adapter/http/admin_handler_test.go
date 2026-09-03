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

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestAdminHandler_ListUsers(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		now := time.Now()
		usecase.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return([]domain.User{
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

		mux, usecase := setupAdminHandlerMux(t)
		usecase.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success with active filter", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		now := time.Now()
		usecase.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p user.AdminListParams) bool {
			return p.Active != nil && *p.Active == true
		})).Return([]domain.User{
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
	})

	t.Run("success with role filter", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		now := time.Now()
		usecase.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p user.AdminListParams) bool {
			return p.Role == "admin"
		})).Return([]domain.User{
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
			Items []listedUserItem `json:"items"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Len(t, got.Items, 1)
		assert.Equal(t, "admin", got.Items[0].Role)
	})
}

func TestAdminHandler_GetUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		userID := uuid.New()
		now := time.Now()
		usecase.EXPECT().GetUser(mock.Anything, userID).Return(&domain.User{
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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, usecase := setupAdminHandlerMux(t)
		userID := uuid.New()
		usecase.EXPECT().GetUser(mock.Anything, userID).Return(nil, errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		userID := uuid.New()
		now := time.Now()
		usecase.EXPECT().AdminUpdate(mock.Anything, userID, "Updated", "Name", (*string)(nil), (*bool)(nil)).
			Return(&domain.User{
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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, usecase := setupAdminHandlerMux(t)

		userID := uuid.New()
		usecase.EXPECT().AdminUpdate(mock.Anything, userID, "Test", "", (*string)(nil), (*bool)(nil)).
			Return(nil, errs.ErrNotFound)

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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, usecase := setupAdminHandlerMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		usecase.EXPECT().UpdateRole(mock.Anything, requesterID, targetID, "admin").Return(nil)

		body, _ := json.Marshal(map[string]any{"role": "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: requesterID,
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminHandlerMux(t)

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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, _ := setupAdminHandlerMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/users/"+targetID.String()+"/role",
			bytes.NewReader([]byte("{invalid")),
		)
		r.Header.Set("Content-Type", "application/json")
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: requesterID,
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

		mux, usecase := setupAdminHandlerMux(t)

		sameID := uuid.New()
		usecase.EXPECT().UpdateRole(mock.Anything, sameID, sameID, "user").
			Return(errs.ErrForbidden)
		body, _ := json.Marshal(map[string]any{"role": "user"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+sameID.String()+"/role", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: sameID,
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

		mux, _ := setupAdminHandlerMux(t)

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
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: requesterID,
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

func TestAdminHandler_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminHandlerMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		usecase.EXPECT().Delete(mock.Anything, requesterID, targetID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+targetID.String(), nil)
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: requesterID,
			Role:   "admin",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminHandlerMux(t)

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

		mux, _ := setupAdminHandlerMux(t)

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

		mux, usecase := setupAdminHandlerMux(t)

		sameID := uuid.New()
		usecase.EXPECT().Delete(mock.Anything, sameID, sameID).Return(errs.ErrForbidden)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+sameID.String(), nil)
		ctx := identity.NewContext(r.Context(), identity.Identity{
			UserID: sameID,
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

func setupAdminHandlerMux(t *testing.T) (*http.ServeMux, *MockUserManager) {
	usecase := NewMockUserManager(t)

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/v1/admin")

	h := NewAdminHandler(usecase)
	admin.HandleFunc("GET /users", h.List)
	admin.HandleFunc("GET /users/{id}", h.GetUser)
	admin.HandleFunc("PUT /users/{id}", h.Update)
	admin.HandleFunc("PUT /users/{id}/role", h.UpdateRole)
	admin.HandleFunc("DELETE /users/{id}", h.Delete)

	return mux, usecase
}

type listedUserItem struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
}
