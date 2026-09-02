package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
)

func TestAuth(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		mid := authMiddleware(authenticator)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid format", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		mid := authMiddleware(authenticator)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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
		mid := authMiddleware(authenticator)

		userID := uuid.New()
		authenticator.EXPECT().Authenticate(mock.Anything, "tok").
			Return(auth.ClaimsView{UserID: userID, Type: "access"}, nil)

		called := false
		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	// The middleware maps whatever the authenticator rejects; which rules it
	// enforces, and why, is auth.Service.Authenticate's business and is tested
	// there. What matters here is that a rejection never reaches the handler
	// and keeps the sentinel's status code.
	t.Run("rejection is mapped and the handler is skipped", func(t *testing.T) {
		for name, err := range map[string]error{
			"invalid token":       auth.ErrInvalidToken,
			"account deactivated": auth.ErrAccountDeactivated,
			"token revoked":       auth.ErrTokenRevoked,
			"invalid credentials": auth.ErrInvalidCredentials,
		} {
			t.Run(name, func(t *testing.T) {
				authenticator := NewMockAuthenticator(t)
				mid := authMiddleware(authenticator)

				authenticator.EXPECT().Authenticate(mock.Anything, "tok").
					Return(auth.ClaimsView{}, err)

				handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
					t.Fatal("handler should not be called")
				}))

				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer tok")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				assert.Equal(t, http.StatusUnauthorized, rec.Code)
			})
		}
	})

	t.Run("a non-sentinel failure is not reported as unauthorized", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		mid := authMiddleware(authenticator)

		authenticator.EXPECT().Authenticate(mock.Anything, "tok").
			Return(auth.ClaimsView{}, assert.AnError)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("success", func(t *testing.T) {
		authenticator := NewMockAuthenticator(t)
		mid := authMiddleware(authenticator)

		userID := uuid.New()
		authenticator.EXPECT().Authenticate(mock.Anything, "valid-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "admin",
			Type:         "access",
			TokenVersion: 3,
		}, nil)

		called := false
		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			uc, ok := middleware.GetUserContext(r.Context())
			if !assert.True(t, ok) {
				return
			}
			assert.Equal(t, middleware.UserContext{
				UserID:       userID,
				Email:        "user@example.com",
				Role:         "admin",
				TokenVersion: 3,
			}, uc)
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
		mid := authMiddleware(authenticator)

		authenticator.EXPECT().Authenticate(mock.Anything, "good-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "a@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)

		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
