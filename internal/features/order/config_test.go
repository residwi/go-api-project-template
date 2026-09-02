package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: every subtest calls t.Setenv, which panics if the test (or
// an ancestor) has called t.Parallel. This file is on the paralleltest
// exclusion list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("rejects a job interval that would hammer the database", func(t *testing.T) {
		t.Setenv("ORDER_JOB_INTERVAL", "1s")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ORDER_JOB_INTERVAL must be at least 5s")
	})

	t.Run("rejects a zero job concurrency that would deadlock the runner", func(t *testing.T) {
		t.Setenv("ORDER_JOB_CONCURRENCY", "0")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ORDER_JOB_CONCURRENCY must be at least 1")
	})

	t.Run("rejects a sub-second rate window that would divide by zero in the limiter", func(t *testing.T) {
		t.Setenv("ORDER_RATE_WINDOW", "500ms")

		_, err := LoadConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ORDER_RATE_WINDOW must be at least 1s")
	})
}
