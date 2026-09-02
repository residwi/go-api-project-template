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
	t.Run("rejects a sub-second rate window that would divide by zero in the limiter", func(t *testing.T) {
		// JWT_SECRET is required:"true"; set it so envconfig gets past that check
		// and actually reaches the rate-window validation this subtest is for.
		t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
		t.Setenv("AUTH_RATE_WINDOW", "500ms")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "AUTH_RATE_WINDOW must be at least 1s")
	})

	t.Run("rejects a bcrypt cost outside bcrypt's valid range", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "test-secret-key-at-least-32-chars-long")
		t.Setenv("BCRYPT_COST", "99")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "BCRYPT_COST must be between")
	})
}
