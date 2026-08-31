package http

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web"
)

func TestHandler_Login(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		service.EXPECT().Login(mock.Anything, "test@example.com", "password123").Return(&domain.TokenPair{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    900,
			User:         user.Profile{Email: "test@example.com"},
		}, nil)

		body, _ := json.Marshal(map[string]any{
			"email":    "test@example.com",
			"password": "password123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, data["access_token"])
		assert.NotEmpty(t, data["refresh_token"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte("invalid-json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error missing email", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		body, _ := json.Marshal(map[string]string{
			"password": "password123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error user not found", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		service.EXPECT().Login(mock.Anything, "notfound@example.com", "password123").
			Return(nil, auth.ErrInvalidCredentials)

		body, _ := json.Marshal(map[string]any{
			"email":    "notfound@example.com",
			"password": "password123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestHandler_Register(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		userID := uuid.New()
		service.EXPECT().Register(mock.Anything, "test@example.com", "password123", "John", "Doe").
			Return(&domain.TokenPair{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				ExpiresIn:    900,
				User: user.Profile{
					ID:        userID,
					Email:     "test@example.com",
					FirstName: "John",
					LastName:  "Doe",
					Role:      "user",
					Active:    true,
				},
			}, nil)

		body, _ := json.Marshal(map[string]any{
			"email":      "test@example.com",
			"password":   "password123",
			"first_name": "John",
			"last_name":  "Doe",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			User         struct {
				Email string `json:"email"`
			} `json:"user"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.NotEmpty(t, got.AccessToken)
		assert.NotEmpty(t, got.RefreshToken)
		assert.Equal(t, "test@example.com", got.User.Email)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader([]byte("invalid-json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error missing email", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		body, _ := json.Marshal(map[string]string{
			"password":   "password123",
			"first_name": "John",
			"last_name":  "Doe",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error duplicate email", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		service.EXPECT().Register(mock.Anything, "test@example.com", "password123", "John", "Doe").
			Return(nil, errs.ErrConflict)

		body, _ := json.Marshal(map[string]any{
			"email":      "test@example.com",
			"password":   "password123",
			"first_name": "John",
			"last_name":  "Doe",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}

func TestHandler_Refresh(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		service.EXPECT().Refresh(mock.Anything, "a-refresh-token").Return(&domain.TokenPair{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    900,
			User:         user.Profile{Email: "test@example.com"},
		}, nil)

		body, _ := json.Marshal(map[string]any{"refresh_token": "a-refresh-token"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, data["access_token"])
		assert.NotEmpty(t, data["refresh_token"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader([]byte("invalid-json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error missing token", func(t *testing.T) {
		t.Parallel()

		mux, _ := newTestMux(t)

		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error invalid token", func(t *testing.T) {
		t.Parallel()

		mux, service := newTestMux(t)

		service.EXPECT().Refresh(mock.Anything, "invalid-token").Return(nil, auth.ErrInvalidToken)

		body, _ := json.Marshal(map[string]any{"refresh_token": "invalid-token"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}

// TokenVersion doubles as revocation state: Refresh rejects a refresh token
// whose version no longer matches the user's, and both it and Active are
// auth-internal and must stay off the wire.
func TestToTokenResponse_OmitsUserInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tp := &domain.TokenPair{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresIn:    900,
		User: user.Profile{
			ID:           userID,
			Email:        "user@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       false,
			TokenVersion: 424242,
		},
	}

	got := toTokenResponse(tp)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(
		t,
		[]string{"access_token", "refresh_token", "expires_in", "user"},
		slices.Collect(maps.Keys(fields)),
		"the token response must expose exactly these fields",
	)

	var userFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["user"], &userFields))
	assert.ElementsMatch(
		t,
		[]string{"id", "email", "first_name", "last_name", "role"},
		slices.Collect(maps.Keys(userFields)),
		"the embedded user must expose exactly these fields",
	)

	assert.NotContains(t, string(raw), "424242",
		"token_version is auth-internal revocation state and must not be serialised")
	assert.NotContains(t, string(raw), `"active"`,
		"active is auth-internal and must not be serialised")
}

func newTestMux(t *testing.T) (http.Handler, *MockAuthManager) {
	service := NewMockAuthManager(t)
	v := validator.New()

	mux := http.NewServeMux()
	api := web.NewRouter(mux).Group("/api")
	h := NewHandler(service, v)
	api.HandleFunc("POST /auth/login", h.Login)
	api.HandleFunc("POST /auth/register", h.Register)
	api.HandleFunc("POST /auth/refresh", h.Refresh)
	return mux, service
}
