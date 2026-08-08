package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/auth/contract"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

func TestService_BuildTokenPair(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	issuer := "test-issuer"
	accessTTL := 15 * time.Minute
	refreshTTL := 24 * time.Hour
	userID := uuid.New()
	user := usercontract.User{
		ID:           userID,
		Email:        "user@example.com",
		Role:         "customer",
		TokenVersion: 1,
	}

	t.Run("success produces valid tokens for both kinds", func(t *testing.T) {
		t.Parallel()

		svc := New(secret, issuer, accessTTL, refreshTTL)

		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)
		assert.NotEmpty(t, pair.AccessToken)
		assert.NotEmpty(t, pair.RefreshToken)
		assert.Equal(t, int(accessTTL/time.Second), pair.ExpiresIn)
		assert.Equal(t, user, pair.User)

		accessClaims, err := svc.ValidateToken(pair.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, contract.Claims{
			UserID:       userID,
			Email:        "user@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, accessClaims)

		refreshClaims, err := svc.ValidateToken(pair.RefreshToken)
		require.NoError(t, err)
		assert.Equal(t, contract.Claims{
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

	secret := "test-secret-key"
	issuer := "test-issuer"
	ttl := 15 * time.Minute
	user := usercontract.User{
		ID:    uuid.New(),
		Email: "user@example.com",
		Role:  "customer",
	}

	t.Run("wrong secret returns error", func(t *testing.T) {
		t.Parallel()

		pair, err := New(secret, issuer, ttl, ttl).BuildTokenPair(user)
		require.NoError(t, err)

		claims, err := New("wrong-secret", issuer, ttl, ttl).ValidateToken(pair.AccessToken)
		assert.Equal(t, contract.Claims{}, claims)
		assert.Error(t, err)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		t.Parallel()

		svc := New(secret, issuer, -1*time.Second, ttl)
		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)

		claims, err := svc.ValidateToken(pair.AccessToken)
		assert.Equal(t, contract.Claims{}, claims)
		assert.Error(t, err)
	})

	t.Run("tampered token returns error", func(t *testing.T) {
		t.Parallel()

		svc := New(secret, issuer, ttl, ttl)
		pair, err := svc.BuildTokenPair(user)
		require.NoError(t, err)

		tampered := pair.AccessToken[:len(pair.AccessToken)-5] + "XXXXX"

		claims, err := svc.ValidateToken(tampered)
		assert.Equal(t, contract.Claims{}, claims)
		assert.Error(t, err)
	})

	t.Run("completely invalid token string", func(t *testing.T) {
		t.Parallel()

		svc := New(secret, issuer, ttl, ttl)

		claims, err := svc.ValidateToken("not-a-token")
		assert.Equal(t, contract.Claims{}, claims)
		assert.Error(t, err)
	})

	t.Run("unexpected signing method returns error", func(t *testing.T) {
		t.Parallel()

		svc := New(secret, issuer, ttl, ttl)

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
		assert.Equal(t, contract.Claims{}, claims)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}
