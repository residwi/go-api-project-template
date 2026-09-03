package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func TestAuth(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)

		handler := Auth(authenticator)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid format", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)

		handler := Auth(authenticator)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "InvalidFormatToken")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("bearer prefix is matched case-insensitively", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		authenticator.EXPECT().Authenticate(mock.Anything, "tok").
			Return(identity.Identity{UserID: uuid.New()}, nil)

		called := false
		handler := Auth(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "bEaReR tok")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	// Which rules reject a caller belongs to whichever Authenticator is wired in;
	// this layer only has to refuse the request and keep the error's status.
	t.Run("an unauthorized rejection is a 401 and never reaches the handler", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		authenticator.EXPECT().Authenticate(mock.Anything, "tok").
			Return(identity.Identity{}, fmt.Errorf("%w: token has been revoked", errs.ErrUnauthorized))

		handler := Auth(authenticator)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("a failure that is not an auth rejection is not downgraded to 401", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		authenticator.EXPECT().Authenticate(mock.Anything, "tok").
			Return(identity.Identity{}, assert.AnError)

		handler := Auth(authenticator)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("success puts the identity in the request context", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)

		want := identity.Identity{UserID: uuid.New(), Role: "admin"}
		authenticator.EXPECT().Authenticate(mock.Anything, "valid-token").Return(want, nil)

		called := false
		handler := Auth(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			got, ok := identity.FromContext(r.Context())
			if !assert.True(t, ok) {
				return
			}
			assert.Equal(t, want, got)
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("downstream logs carry the user id", func(t *testing.T) {
		var buf bytes.Buffer
		log := slog.New(logger.ContextHandler{Handler: slog.NewJSONHandler(&buf, nil)})

		userID := uuid.New()
		authenticator := NewMockAuthenticator(t)
		authenticator.EXPECT().Authenticate(mock.Anything, "good-token").
			Return(identity.Identity{UserID: userID, Role: "user"}, nil)

		handler := Auth(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.InfoContext(r.Context(), "downstream")
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer good-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)

		var entry map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		assert.Equal(t, userID.String(), entry["user_id"])
	})
}
