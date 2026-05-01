package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/middleware"
	mocks "github.com/residwi/go-api-project-template/mocks/middleware"
)

func TestAuth(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		handler := mid(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid format", func(t *testing.T) {
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		tokenValidator.EXPECT().ValidateToken("bad-token").Return(nil, errors.New("invalid"))

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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		tokenValidator.EXPECT().ValidateToken("refresh-token").Return(&middleware.TokenClaims{
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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(&middleware.TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(middleware.UserStatusResult{}, errors.New("db error"))

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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(&middleware.TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(middleware.UserStatusResult{
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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(&middleware.TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(middleware.UserStatusResult{
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
		tokenValidator := mocks.NewMockTokenValidator(t)
		userStatus := mocks.NewMockUserStatusChecker(t)
		mid := middleware.Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.EXPECT().ValidateToken("valid-token").Return(&middleware.TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "admin",
			Type:         "access",
			TokenVersion: 3,
		}, nil)
		userStatus.EXPECT().CheckStatus(mock.Anything, userID).Return(middleware.UserStatusResult{
			Active:       true,
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
}
