package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRequireUser(t *testing.T) {
	t.Run("returns the user when present in context", func(t *testing.T) {
		want := UserContext{UserID: uuid.New(), Email: "a@example.com", Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(SetUserContext(r.Context(), want))
		w := httptest.NewRecorder()

		got, ok := RequireUser(w, r)

		require.True(t, ok)
		assert.Equal(t, want, got)
		assert.Equal(t, http.StatusOK, w.Code) // nothing written when authenticated
	})

	t.Run("writes 401 when no user in context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		_, ok := RequireUser(w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestAuth(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		tokenValidator.On("ValidateToken", "bad-token").Return(nil, errors.New("invalid"))

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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		tokenValidator.On("ValidateToken", "refresh-token").Return(&TokenClaims{
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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.On("ValidateToken", "valid-token").Return(&TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.On("CheckStatus", mock.Anything, userID).Return(UserStatusResult{}, errors.New("db error"))

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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.On("ValidateToken", "valid-token").Return(&TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.On("CheckStatus", mock.Anything, userID).Return(UserStatusResult{
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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.On("ValidateToken", "valid-token").Return(&TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "user",
			Type:         "access",
			TokenVersion: 1,
		}, nil)
		userStatus.On("CheckStatus", mock.Anything, userID).Return(UserStatusResult{
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
		tokenValidator := newStubTokenValidator(t)
		userStatus := newStubUserStatusChecker(t)
		mid := Auth(tokenValidator, userStatus)

		userID := uuid.New()
		tokenValidator.On("ValidateToken", "valid-token").Return(&TokenClaims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "admin",
			Type:         "access",
			TokenVersion: 3,
		}, nil)
		userStatus.On("CheckStatus", mock.Anything, userID).Return(UserStatusResult{
			Active:       true,
			TokenVersion: 3,
		}, nil)

		called := false
		handler := mid(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			uc, ok := GetUserContext(r.Context())
			if !assert.True(t, ok) {
				return
			}
			assert.Equal(t, UserContext{
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

// stubTokenValidator and stubUserStatusChecker replace the generated
// mocks/middleware package for this test file specifically. TokenValidator and
// UserStatusChecker are declared in this package, and mocks/middleware imports
// this package to reference *TokenClaims/UserStatusResult -- fine for an
// external middleware_test package, but an import cycle once this file moved
// in-package. Hand-rolling directly against testify's mock.Mock keeps the same
// On/Return/AssertExpectations semantics the generated mocks used.
type stubTokenValidator struct{ mock.Mock }

func newStubTokenValidator(t *testing.T) *stubTokenValidator {
	t.Helper()
	m := &stubTokenValidator{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *stubTokenValidator) ValidateToken(tokenString string) (*TokenClaims, error) {
	args := m.Called(tokenString)
	claims, _ := args.Get(0).(*TokenClaims)
	return claims, args.Error(1)
}

type stubUserStatusChecker struct{ mock.Mock }

func newStubUserStatusChecker(t *testing.T) *stubUserStatusChecker {
	t.Helper()
	m := &stubUserStatusChecker{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *stubUserStatusChecker) CheckStatus(ctx context.Context, userID uuid.UUID) (UserStatusResult, error) {
	args := m.Called(ctx, userID)
	result, _ := args.Get(0).(UserStatusResult)
	return result, args.Error(1)
}
