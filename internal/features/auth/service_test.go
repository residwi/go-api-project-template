package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

func TestService_Login(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		userID := uuid.New()
		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
		creds := user.Credentials{
			ID:           userID,
			Email:        "test@example.com",
			PasswordHash: string(hash),
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(creds, nil)

		resp, err := New(newTestConfig(), users).
			Login(context.Background(), "test@example.com", "password123")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, user.Profile{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}, resp.User)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
		users.EXPECT().GetByEmail(mock.Anything, "inactive@example.com").Return(user.Credentials{
			ID:           uuid.New(),
			Email:        "inactive@example.com",
			PasswordHash: string(hash),
			Active:       false,
		}, nil)

		resp, err := New(newTestConfig(), users).
			Login(context.Background(), "inactive@example.com", "password123")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errs.ErrUnauthorized)
	})

	t.Run("wrong password returns ErrInvalidCredentials", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(user.Credentials{
			ID:           uuid.New(),
			Email:        "test@example.com",
			PasswordHash: string(hash),
			Active:       true,
		}, nil)

		resp, err := New(newTestConfig(), users).
			Login(context.Background(), "test@example.com", "wrong-password")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("user not found returns ErrInvalidCredentials", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		users.EXPECT().GetByEmail(mock.Anything, "notfound@example.com").
			Return(user.Credentials{}, errors.New("not found"))

		resp, err := New(newTestConfig(), users).
			Login(context.Background(), "notfound@example.com", "password123")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
}

func TestService_Register(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		userID := uuid.New()
		createdUser := user.Profile{
			ID:        userID,
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Role:      "customer",
			Active:    true,
		}

		users.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p user.NewUser) bool {
			return p.Email == "test@example.com" &&
				p.FirstName == "John" &&
				p.LastName == "Doe" &&
				bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte("password123")) == nil
		})).Return(createdUser, nil)

		resp, err := New(newTestConfig(), users).
			Register(context.Background(), "test@example.com", "password123", "John", "Doe")

		require.NoError(t, err)
		assert.Equal(t, userID, resp.User.ID)
		assert.Equal(t, "test@example.com", resp.User.Email)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("Create error propagates", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)

		users.EXPECT().Create(mock.Anything, mock.Anything).
			Return(user.Profile{}, errs.ErrConflict)

		resp, err := New(newTestConfig(), users).
			Register(context.Background(), "dup@example.com", "password123", "John", "Doe")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("password exceeding 72 bytes is a bad request, not a 500", func(t *testing.T) {
		t.Parallel()

		longPassword := strings.Repeat("a", 73)

		var users UserDirectory
		resp, err := New(newTestConfig(), users).
			Register(context.Background(), "test@example.com", longPassword, "John", "Doe")

		assert.Nil(t, resp)
		require.Error(t, err)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})
}

func TestService_Refresh(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)
		svc := New(newTestConfig(), users)

		userID := uuid.New()
		user := user.Profile{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}

		// Mint a real refresh token the way Login/Register would, so this
		// exercises Refresh's own ValidateToken rather than a mock -- once
		// token folded into Service there is no longer a separate
		// TokenValidator to inject one behind.
		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(user, nil)

		resp, err := svc.Refresh(context.Background(), pair.RefreshToken)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("invalid token returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		var users UserDirectory
		svc := New(newTestConfig(), users)

		resp, err := svc.Refresh(context.Background(), "not-a-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("access token instead of refresh returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		var users UserDirectory
		svc := New(newTestConfig(), users)

		pair, err := svc.BuildTokenPair(user.Profile{
			ID: uuid.New(), Email: "test@example.com", Role: "customer", TokenVersion: 1,
		})
		require.NoError(t, err)

		resp, err := svc.Refresh(context.Background(), pair.AccessToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)
		svc := New(newTestConfig(), users)

		userID := uuid.New()
		pair, err := svc.BuildTokenPair(user.Profile{
			ID: userID, Email: "test@example.com", Role: "customer", TokenVersion: 1,
		})
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(user.Profile{
			ID:           userID,
			Email:        "test@example.com",
			Active:       false,
			TokenVersion: 1,
		}, nil)

		resp, err := svc.Refresh(context.Background(), pair.RefreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errs.ErrUnauthorized)
	})

	t.Run("token version mismatch returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)
		svc := New(newTestConfig(), users)

		userID := uuid.New()
		pair, err := svc.BuildTokenPair(user.Profile{
			ID: userID, Email: "test@example.com", Role: "customer", TokenVersion: 1,
		})
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(user.Profile{
			ID:           userID,
			Email:        "test@example.com",
			Active:       true,
			TokenVersion: 2,
		}, nil)

		resp, err := svc.Refresh(context.Background(), pair.RefreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("GetByID error propagates", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserDirectory(t)
		svc := New(newTestConfig(), users)

		userID := uuid.New()
		pair, err := svc.BuildTokenPair(user.Profile{
			ID: userID, Email: "test@example.com", Role: "customer", TokenVersion: 1,
		})
		require.NoError(t, err)

		dbErr := errors.New("database connection lost")
		users.EXPECT().GetByID(mock.Anything, userID).Return(user.Profile{}, dbErr)

		resp, err := svc.Refresh(context.Background(), pair.RefreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_BuildTokenPair(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	user := user.Profile{
		ID:           userID,
		Email:        "user@example.com",
		Role:         "customer",
		TokenVersion: 1,
	}

	t.Run("success produces valid tokens for both kinds", func(t *testing.T) {
		t.Parallel()

		cfg := newTestConfig()
		var users UserDirectory
		svc := New(cfg, users)

		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)
		assert.NotEmpty(t, pair.AccessToken)
		assert.NotEmpty(t, pair.RefreshToken)
		assert.Equal(t, int(cfg.AccessTokenTTL/time.Second), pair.ExpiresIn)
		assert.Equal(t, user, pair.User)

		accessClaims, err := svc.ValidateToken(pair.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, accessClaims)

		refreshClaims, err := svc.ValidateToken(pair.RefreshToken)
		require.NoError(t, err)
		assert.Equal(t, ClaimsView{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, refreshClaims)
	})
}

func TestService_ValidateToken(t *testing.T) {
	t.Parallel()

	user := user.Profile{
		ID:    uuid.New(),
		Email: "user@example.com",
		Role:  "customer",
	}

	t.Run("wrong secret returns error", func(t *testing.T) {
		t.Parallel()

		cfg := newTestConfig()
		var users UserDirectory
		pair, err := New(cfg, users).BuildTokenPair(user)
		require.NoError(t, err)

		wrongCfg := cfg
		wrongCfg.Secret = "wrong-secret"
		claims, err := New(wrongCfg, users).ValidateToken(pair.AccessToken)
		assert.Equal(t, ClaimsView{}, claims)
		assert.Error(t, err)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		t.Parallel()

		cfg := newTestConfig()
		cfg.AccessTokenTTL = -1 * time.Second
		var users UserDirectory
		svc := New(cfg, users)
		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)

		claims, err := svc.ValidateToken(pair.AccessToken)
		assert.Equal(t, ClaimsView{}, claims)
		assert.Error(t, err)
	})

	t.Run("tampered token returns error", func(t *testing.T) {
		t.Parallel()

		var users UserDirectory
		svc := New(newTestConfig(), users)
		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)

		tampered := pair.AccessToken[:len(pair.AccessToken)-5] + "XXXXX"

		claims, err := svc.ValidateToken(tampered)
		assert.Equal(t, ClaimsView{}, claims)
		assert.Error(t, err)
	})

	t.Run("completely invalid token string", func(t *testing.T) {
		t.Parallel()

		var users UserDirectory
		svc := New(newTestConfig(), users)

		claims, err := svc.ValidateToken("not-a-token")
		assert.Equal(t, ClaimsView{}, claims)
		assert.Error(t, err)
	})

	t.Run("unexpected signing method returns error", func(t *testing.T) {
		t.Parallel()

		var users UserDirectory
		svc := New(newTestConfig(), users)

		tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"user_id": uuid.New().String(),
			"email":   "user@example.com",
			"role":    "customer",
			"typ":     "access",
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		})
		tokenString, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		claims, err := svc.ValidateToken(tokenString)
		assert.Equal(t, ClaimsView{}, claims)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}

// newTestConfig gives every Service test the same secret, issuer and TTLs
// token/usecase_test.go used to hard-code per test; subtests that need a
// different value copy this and override just that field.
func newTestConfig() Config {
	return Config{
		Secret:          "test-secret-key",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}
}
