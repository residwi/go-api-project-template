package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/auth"
)

func TestGenerateTokenPair(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	accessTTL := 15 * time.Minute
	refreshTTL := 24 * time.Hour
	userID := uuid.New()
	claims := auth.Claims{
		UserID:       userID,
		Email:        "user@example.com",
		Role:         "customer",
		Type:         "",
		TokenVersion: 1,
	}

	t.Run("success produces valid tokens", func(t *testing.T) {
		accessToken, refreshToken, err := auth.GenerateTokenPair(secret, issuer, accessTTL, refreshTTL, claims)
		require.NoError(t, err)
		assert.NotEmpty(t, accessToken)
		assert.NotEmpty(t, refreshToken)

		accessClaims, err := auth.ValidateToken(accessToken, secret)
		require.NoError(t, err)
		assert.Equal(t, &auth.Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, accessClaims)

		refreshClaims, err := auth.ValidateToken(refreshToken, secret)
		require.NoError(t, err)
		assert.Equal(t, &auth.Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, refreshClaims)
	})

	t.Run("claims roundtrip preserves all fields", func(t *testing.T) {
		accessToken, _, err := auth.GenerateTokenPair(secret, issuer, accessTTL, refreshTTL, claims)
		require.NoError(t, err)

		got, err := auth.ValidateToken(accessToken, secret)
		require.NoError(t, err)

		expected := &auth.Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}
		assert.Equal(t, expected, got)
	})
}

func TestValidateToken(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	ttl := 15 * time.Minute
	claims := auth.Claims{
		UserID:       uuid.New(),
		Email:        "user@example.com",
		Role:         "customer",
		TokenVersion: 1,
	}

	t.Run("wrong secret returns error", func(t *testing.T) {
		accessToken, _, err := auth.GenerateTokenPair(secret, issuer, ttl, ttl, claims)
		require.NoError(t, err)

		got, err := auth.ValidateToken(accessToken, "wrong-secret")
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		accessToken, _, err := auth.GenerateTokenPair(secret, issuer, -1*time.Second, ttl, claims)
		require.NoError(t, err)

		got, err := auth.ValidateToken(accessToken, secret)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("tampered token returns error", func(t *testing.T) {
		accessToken, _, err := auth.GenerateTokenPair(secret, issuer, ttl, ttl, claims)
		require.NoError(t, err)

		// Flip multiple characters in the signature to ensure invalidation
		tampered := accessToken[:len(accessToken)-5] + "XXXXX"

		got, err := auth.ValidateToken(tampered, secret)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("completely invalid token string", func(t *testing.T) {
		got, err := auth.ValidateToken("not-a-token", secret)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("unexpected signing method returns error", func(t *testing.T) {
		// Create a token with "none" signing method — ValidateToken expects HMAC.
		token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"user_id": uuid.New().String(),
			"email":   "user@example.com",
			"role":    "customer",
			"typ":     "access",
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		})
		tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		got, err := auth.ValidateToken(tokenString, secret)
		assert.Nil(t, got)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}
