package http

import (
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
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/query"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_ListUsers(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminMux(t)

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

		mux, usecase := setupAdminMux(t)
		usecase.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success with active filter", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupAdminMux(t)

		now := time.Now()
		usecase.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p query.Params) bool {
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

		mux, usecase := setupAdminMux(t)

		now := time.Now()
		usecase.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p query.Params) bool {
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

		mux, usecase := setupAdminMux(t)

		userID := uuid.New()
		now := time.Now()
		usecase.EXPECT().GetByID(mock.Anything, userID).Return(&domain.User{
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

		mux, _ := setupAdminMux(t)

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

		mux, usecase := setupAdminMux(t)
		userID := uuid.New()
		usecase.EXPECT().GetByID(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
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

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockUserLister) {
	usecase := NewMockUserLister(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	ah := NewAdmin(usecase)
	admin.HandleFunc("GET /users", ah.List)
	admin.HandleFunc("GET /users/{id}", ah.Get)

	return mux, usecase
}

type listedUserItem struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
}
