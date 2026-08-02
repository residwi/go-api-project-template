package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	userhttp "github.com/residwi/go-api-project-template/internal/modules/user/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	userMocks "github.com/residwi/go-api-project-template/mocks/user"
)

func setupUserMux(t *testing.T) (*http.ServeMux, *userMocks.MockRepository) {
	repo := userMocks.NewMockRepository(t)
	svc := user.NewService(repo, nil)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	userhttp.RegisterRoutes(authed, admin, userhttp.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo
}

func TestHandler_GetProfile(t *testing.T) {
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
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

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

func TestHandler_UpdateProfile(t *testing.T) {
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

		body, _ := json.Marshal(map[string]any{
			"first_name": "Jane",
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
		repo.EXPECT().GetByID(mock.Anything, userID).Return(nil, apperror.ErrNotFound)

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
