package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
)

const (
	testSecret = "test-secret-key"
	testIssuer = "test-issuer"
)

func TestCodec_Parse(t *testing.T) {
	t.Parallel()

	claims := domain.Claims{
		UserID:       uuid.New(),
		Email:        "user@example.com",
		Role:         "customer",
		Type:         "access",
		TokenVersion: 1,
	}

	t.Run("round trips every claim it was given", func(t *testing.T) {
		t.Parallel()

		codec := New(testSecret, testIssuer)
		signed, err := codec.Sign(claims, 15*time.Minute)
		require.NoError(t, err)

		got, err := codec.Parse(signed)

		require.NoError(t, err)
		assert.Equal(t, claims, got)
	})

	t.Run("rejects a token signed with another secret", func(t *testing.T) {
		t.Parallel()

		signed, err := New(testSecret, testIssuer).Sign(claims, 15*time.Minute)
		require.NoError(t, err)

		got, err := New("wrong-secret", testIssuer).Parse(signed)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a token issued by someone else", func(t *testing.T) {
		t.Parallel()

		signed, err := New(testSecret, "other-issuer").Sign(claims, 15*time.Minute)
		require.NoError(t, err)

		got, err := New(testSecret, testIssuer).Parse(signed)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects an expired token", func(t *testing.T) {
		t.Parallel()

		codec := New(testSecret, testIssuer)
		signed, err := codec.Sign(claims, -1*time.Second)
		require.NoError(t, err)

		got, err := codec.Parse(signed)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a tampered token", func(t *testing.T) {
		t.Parallel()

		codec := New(testSecret, testIssuer)
		signed, err := codec.Sign(claims, 15*time.Minute)
		require.NoError(t, err)

		got, err := codec.Parse(signed[:len(signed)-5] + "XXXXX")

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a string that is not a token", func(t *testing.T) {
		t.Parallel()

		got, err := New(testSecret, testIssuer).Parse("not-a-token")

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	// alg=none is the classic JWT forgery: without the HMAC check a caller could
	// hand us unsigned claims and be believed.
	t.Run("rejects an unsigned token", func(t *testing.T) {
		t.Parallel()

		tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"user_id": uuid.New().String(),
			"email":   "user@example.com",
			"role":    "customer",
			"typ":     "access",
			"iss":     testIssuer,
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		})
		signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		got, err := New(testSecret, testIssuer).Parse(signed)

		assert.Equal(t, domain.Claims{}, got)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}
