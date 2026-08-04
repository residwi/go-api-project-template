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
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	authMocks "github.com/residwi/go-api-project-template/mocks/auth"
)

func TestHandler_Register(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, users := newTestMux(t)

		userID := uuid.New()
		users.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p auth.CreateUserParams) bool {
			return p.Email == "test@example.com" &&
				p.FirstName == "John" &&
				p.LastName == "Doe" &&
				bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte("password123")) == nil
		})).Return(auth.UserResult{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Role:      "user",
			Active:    true,
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

		mux, users := newTestMux(t)

		users.EXPECT().Create(mock.Anything, mock.Anything).Return(auth.UserResult{}, apperror.ErrConflict)

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

func TestHandler_Login(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, users := newTestMux(t)

		userID := uuid.New()
		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(auth.UserCredentials{
			ID:           userID,
			Email:        "test@example.com",
			PasswordHash: string(hash),
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
			TokenVersion: 1,
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

		mux, users := newTestMux(t)

		users.EXPECT().GetByEmail(mock.Anything, "notfound@example.com").Return(auth.UserCredentials{}, apperror.ErrNotFound)

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

func TestHandler_RefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, users := newTestMux(t)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "user",
			TokenVersion: 1,
		}
		_, refreshToken, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(auth.UserResult{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
			TokenVersion: 1,
		}, nil)

		body, _ := json.Marshal(map[string]any{"refresh_token": refreshToken})

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

		mux, _ := newTestMux(t)

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

// TokenVersion doubles as revocation state: auth.Service.RefreshToken rejects
// a refresh token whose version no longer matches the user's.
func TestToTokenResponse_OmitsUserInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tp := &auth.TokenPair{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresIn:    900,
		User: auth.UserResult{
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
	assert.ElementsMatch(t, []string{"access_token", "refresh_token", "expires_in", "user"}, slices.Collect(maps.Keys(fields)),
		"the token response must expose exactly these fields")

	var userFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["user"], &userFields))
	assert.ElementsMatch(t, []string{"id", "email", "first_name", "last_name", "role"}, slices.Collect(maps.Keys(userFields)),
		"the embedded user must expose exactly these fields")

	assert.NotContains(t, string(raw), "424242",
		"token_version is auth-internal revocation state and must not be serialised")
	assert.NotContains(t, string(raw), `"active"`,
		"active is auth-internal and must not be serialised")
}

func newTestMux(t *testing.T) (http.Handler, *authMocks.MockUserProvider) {
	users := authMocks.NewMockUserProvider(t)
	svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	RegisterRoutes(api, RouteDeps{
		Validator: v,
		Service:   svc,
	})
	return mux, users
}
