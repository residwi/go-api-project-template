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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Register(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := newTestMux(t)

		userID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, mock.Anything).Return(&domain.TokenPair{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    900,
			User: usercontract.User{
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

		mux, cmd := newTestMux(t)

		cmd.EXPECT().Execute(mock.Anything, mock.Anything).Return(nil, apperror.ErrConflict)

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

// TokenVersion doubles as revocation state; Active is auth-internal too. Both
// must stay off the wire.
func TestToTokenResponse_OmitsUserInternalFields(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tp := &domain.TokenPair{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresIn:    900,
		User: usercontract.User{
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

func newTestMux(t *testing.T) (http.Handler, *MockRegisterer) {
	cmd := NewMockRegisterer(t)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	api.HandleFunc("POST /auth/register", New(cmd, v).Register)
	return mux, cmd
}
