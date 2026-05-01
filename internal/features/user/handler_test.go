package user_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	userMocks "github.com/residwi/go-api-project-template/mocks/user"
)

func setupUserMux(t *testing.T) (*http.ServeMux, *userMocks.MockRepository) {
	repo := userMocks.NewMockRepository(t)
	svc := user.NewService(repo, nil, nil)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	user.RegisterRoutes(authed, admin, user.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo
}

func TestPublicHandler_GetProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupUserMux(t)

		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(&user.User{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
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
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
		}{Email: "test@example.com", FirstName: "John"}, got)
	})

	t.Run("missing auth context", func(t *testing.T) {
		mux, _ := setupUserMux(t)

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
		mux, repo := setupUserMux(t)
		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestPublicHandler_UpdateProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupUserMux(t)

		userID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(&user.User{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Role:      "user",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(user.UpdateProfileRequest{
			FirstName: "Jane",
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
		mux, _ := setupUserMux(t)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/users/me", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _ := setupUserMux(t)
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
		mux, repo := setupUserMux(t)
		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, core.ErrNotFound)

		body, _ := json.Marshal(user.UpdateProfileRequest{FirstName: "Jane"})
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

func TestAdminHandler_ListUsers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.NotNil(t, data["pagination"])
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo := setupUserMux(t)
		repo.EXPECT().List(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_GetUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		mux, repo := setupUserMux(t)
		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_UpdateUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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

		body, _ := json.Marshal(user.AdminUpdateUserRequest{
			FirstName: "Updated",
			LastName:  "Name",
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
		mux, _ := setupUserMux(t)

		body, _ := json.Marshal(user.AdminUpdateUserRequest{FirstName: "Test"})

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
		mux, _ := setupUserMux(t)

		userID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+userID.String(), bytes.NewReader([]byte("{invalid")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("not found", func(t *testing.T) {
		mux, repo := setupUserMux(t)

		userID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, core.ErrNotFound)

		body, _ := json.Marshal(user.AdminUpdateUserRequest{FirstName: "Test"})

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
	t.Run("success", func(t *testing.T) {
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

		body, _ := json.Marshal(user.UpdateRoleRequest{Role: "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+targetID.String()+"/role", bytes.NewReader(body))
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
		mux, _ := setupUserMux(t)

		body, _ := json.Marshal(user.UpdateRoleRequest{Role: "admin"})

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
		mux, _ := setupUserMux(t)

		targetID := uuid.New()
		body, _ := json.Marshal(user.UpdateRoleRequest{Role: "admin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+targetID.String()+"/role", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "authentication required", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _ := setupUserMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+targetID.String()+"/role", bytes.NewReader([]byte("{invalid")))
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
		mux, _ := setupUserMux(t)

		sameID := uuid.New()
		body, _ := json.Marshal(user.UpdateRoleRequest{Role: "user"})

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
		mux, _ := setupUserMux(t)

		requesterID := uuid.New()
		targetID := uuid.New()
		body, _ := json.Marshal(user.UpdateRoleRequest{Role: "superadmin"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+targetID.String()+"/role", bytes.NewReader(body))
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
	t.Run("success", func(t *testing.T) {
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
	t.Run("success with active filter", func(t *testing.T) {
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
	})

	t.Run("success with role filter", func(t *testing.T) {
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
				Role string `json:"role"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Len(t, got.Items, 1)
		assert.Equal(t, "admin", got.Items[0].Role)
	})
}
