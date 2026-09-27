package promotion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: every subtest calls t.Setenv, which panics if the test (or
// an ancestor) has called t.Parallel. This file is on the paralleltest
// exclusion list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("defaults to its own limit, independent of the auth limiter", func(t *testing.T) {
		t.Setenv("AUTH_RATE_LIMIT", "1000")

		cfg, err := LoadConfig()

		require.NoError(t, err)
		assert.Equal(t, Config{RateLimit: 10, RateWindow: time.Minute}, cfg)
	})

	t.Run("enforces a minimum 1 second rate limit window", func(t *testing.T) {
		t.Setenv("PROMOTION_RATE_WINDOW", "500ms")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PROMOTION_RATE_WINDOW must be at least 1s")
	})
}
