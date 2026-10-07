package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: this test calls t.Setenv, which panics if the test (or an
// ancestor) has called t.Parallel. This file is on the paralleltest exclusion
// list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("enforces a minimum 1 second rate limit window", func(t *testing.T) {
		// JWT_SECRET is required:"true"; set it so envconfig gets past that check
		// and actually reaches the rate-window validation this subtest is for.
		t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
		t.Setenv("AUTH_RATE_WINDOW", "500ms")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "AUTH_RATE_WINDOW must be at least 1s")
	})

	t.Run("rejects a JWT secret shorter than the strength floor", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "too-short-secret")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "JWT_SECRET must be at least")
	})

	t.Run("rejects the .env.example placeholder secret", func(t *testing.T) {
		t.Setenv("JWT_SECRET", placeholderSecret)

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "JWT_SECRET must not be the .env.example placeholder")
	})

	t.Run("loads a strong non-placeholder secret", func(t *testing.T) {
		secret := "a-sufficiently-long-and-unique-jwt-secret"
		t.Setenv("JWT_SECRET", secret)

		cfg, err := LoadConfig()

		require.NoError(t, err)
		assert.Equal(t, secret, cfg.Secret)
	})
}
