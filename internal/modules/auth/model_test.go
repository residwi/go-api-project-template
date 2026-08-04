package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTokenPair(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	accessTTL := 15 * time.Minute
	refreshTTL := 24 * time.Hour
	userID := uuid.New()
	claims := Claims{
		UserID:       userID,
		Email:        "user@example.com",
		Role:         "customer",
		Type:         "",
		TokenVersion: 1,
	}

	t.Run("success produces valid tokens", func(t *testing.T) {
		accessToken, refreshToken, err := GenerateTokenPair(secret, issuer, accessTTL, refreshTTL, claims)
		require.NoError(t, err)
		assert.NotEmpty(t, accessToken)
		assert.NotEmpty(t, refreshToken)

		accessClaims, err := ValidateToken(accessToken, secret, issuer)
		require.NoError(t, err)
		assert.Equal(t, &Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, accessClaims)

		refreshClaims, err := ValidateToken(refreshToken, secret, issuer)
		require.NoError(t, err)
		assert.Equal(t, &Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, refreshClaims)
	})

	t.Run("claims roundtrip preserves all fields", func(t *testing.T) {
		accessToken, _, err := GenerateTokenPair(secret, issuer, accessTTL, refreshTTL, claims)
		require.NoError(t, err)

		got, err := ValidateToken(accessToken, secret, issuer)
		require.NoError(t, err)

		expected := &Claims{
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
	claims := Claims{
		UserID:       uuid.New(),
		Email:        "user@example.com",
		Role:         "customer",
		TokenVersion: 1,
	}

	t.Run("wrong secret returns error", func(t *testing.T) {
		accessToken, _, err := GenerateTokenPair(secret, issuer, ttl, ttl, claims)
		require.NoError(t, err)

		got, err := ValidateToken(accessToken, "wrong-secret", issuer)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		accessToken, _, err := GenerateTokenPair(secret, issuer, -1*time.Second, ttl, claims)
		require.NoError(t, err)

		got, err := ValidateToken(accessToken, secret, issuer)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("tampered token returns error", func(t *testing.T) {
		accessToken, _, err := GenerateTokenPair(secret, issuer, ttl, ttl, claims)
		require.NoError(t, err)

		tampered := accessToken[:len(accessToken)-5] + "XXXXX"

		got, err := ValidateToken(tampered, secret, issuer)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("completely invalid token string", func(t *testing.T) {
		got, err := ValidateToken("not-a-token", secret, issuer)
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	t.Run("unexpected signing method returns error", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"user_id": uuid.New().String(),
			"email":   "user@example.com",
			"role":    "customer",
			"typ":     "access",
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		})
		tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		got, err := ValidateToken(tokenString, secret, issuer)
		assert.Nil(t, got)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}
