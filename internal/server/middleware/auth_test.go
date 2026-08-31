package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
	webmw "github.com/residwi/go-api-project-template/internal/platform/web/middleware"
)

func TestAuth(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid format", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "InvalidFormatToken")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		tokenValidator.EXPECT().ValidateToken("bad-token").Return(auth.ClaimsView{}, errors.New("invalid"))

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("wrong token type", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		tokenValidator.EXPECT().ValidateToken("refresh-token").Return(auth.ClaimsView{
			UserID: uuid.New(),
			Email:  "user@example.com",
			Role:   "user",
			Type:   "refresh",
		}, nil)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer refresh-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("check status error returns internal error", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).
			Return(user.AccountStatus{}, errors.New("db error"))

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("inactive user", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(user.AccountStatus{
			Active:       false,
			TokenVersion: 1,
		}, nil)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("token version mismatch", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(user.AccountStatus{
			Active:       true,
			TokenVersion: 2,
		}, nil)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("success", func(t *testing.T) {
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "admin",
			Type:         "access",
			TokenVersion: 3,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(user.AccountStatus{
			Active:       true,
			TokenVersion: 3,
		}, nil)

		called := false
		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			uc, ok := webmw.GetUserContext(r.Context())
			if !assert.True(t, ok) {
				return
			}
			assert.Equal(t, webmw.UserContext{
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
		tokenValidator := NewMockTokenValidator(t)
		userStatus := NewMockUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		tokenValidator.EXPECT().ValidateToken("good-token").Return(auth.ClaimsView{
			UserID:       userID,
			Email:        "a@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).
			Return(user.AccountStatus{Active: true, TokenVersion: 1}, nil)

		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.InfoContext(r.Context(), "downstream work")
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer good-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)

		var record map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
		assert.Equal(t, userID.String(), record["user_id"])
	})
}
