package jwt

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

func TestTokens_Verify(t *testing.T) {
	t.Parallel()

	claims := domain.Claims{
		UserID:       uuid.New(),
		Role:         "customer",
		TokenVersion: 1,
	}

	t.Run("round trips every claim it was given", func(t *testing.T) {
		t.Parallel()

		tokens := New(testSecret, testIssuer)
		signed, err := tokens.Issue(claims, domain.AccessToken, 15*time.Minute)
		require.NoError(t, err)

		got, err := tokens.Verify(signed, domain.AccessToken)

		require.NoError(t, err)
		assert.Equal(t, claims, got)
	})

	t.Run("rejects a token signed with another secret", func(t *testing.T) {
		t.Parallel()

		signed, err := New(testSecret, testIssuer).Issue(claims, domain.AccessToken, 15*time.Minute)
		require.NoError(t, err)

		got, err := New("wrong-secret", testIssuer).Verify(signed, domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a token issued by someone else", func(t *testing.T) {
		t.Parallel()

		signed, err := New(testSecret, "other-issuer").Issue(claims, domain.AccessToken, 15*time.Minute)
		require.NoError(t, err)

		got, err := New(testSecret, testIssuer).Verify(signed, domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects an expired token", func(t *testing.T) {
		t.Parallel()

		tokens := New(testSecret, testIssuer)
		signed, err := tokens.Issue(claims, domain.AccessToken, -1*time.Second)
		require.NoError(t, err)

		got, err := tokens.Verify(signed, domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a tampered token", func(t *testing.T) {
		t.Parallel()

		tokens := New(testSecret, testIssuer)
		signed, err := tokens.Issue(claims, domain.AccessToken, 15*time.Minute)
		require.NoError(t, err)

		got, err := tokens.Verify(signed[:len(signed)-5]+"XXXXX", domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	t.Run("rejects a string that is not a token", func(t *testing.T) {
		t.Parallel()

		got, err := New(testSecret, testIssuer).Verify("not-a-token", domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		assert.Error(t, err)
	})

	// A refresh token is long-lived and never sent to normal endpoints; if it
	// could authenticate one, stealing it would be equivalent to stealing a
	// password. Verify refuses on kind, so no caller can forget to check.
	t.Run("refuses a refresh token where an access token is wanted", func(t *testing.T) {
		t.Parallel()

		tokens := New(testSecret, testIssuer)
		signed, err := tokens.Issue(claims, domain.RefreshToken, 24*time.Hour)
		require.NoError(t, err)

		got, err := tokens.Verify(signed, domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		require.Error(t, err)
		assert.ErrorContains(t, err, "want \"access\"")
	})

	t.Run("accepts a refresh token where one is wanted", func(t *testing.T) {
		t.Parallel()

		tokens := New(testSecret, testIssuer)
		signed, err := tokens.Issue(claims, domain.RefreshToken, 24*time.Hour)
		require.NoError(t, err)

		got, err := tokens.Verify(signed, domain.RefreshToken)

		require.NoError(t, err)
		assert.Equal(t, claims, got)
	})

	// alg=none is the classic JWT forgery: without the HMAC check a caller could
	// hand us unsigned claims and be believed.
	t.Run("rejects an unsigned token", func(t *testing.T) {
		t.Parallel()

		tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"user_id": uuid.New().String(),
			"email":   "user@example.com",
			"role":    "customer",
			"kind":    "access",
			"iss":     testIssuer,
			"exp":     time.Now().Add(15 * time.Minute).Unix(),
		})
		signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		got, err := New(testSecret, testIssuer).Verify(signed, domain.AccessToken)

		assert.Equal(t, domain.Claims{}, got)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected signing method")
	})
}
