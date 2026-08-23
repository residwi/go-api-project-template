package order

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: the second subtest calls t.Setenv, which panics if the test
// (or an ancestor) has called t.Parallel. This file is on the paralleltest
// exclusion list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("rejects a lease that outlives the stale-processing threshold", func(t *testing.T) {
		_, err := LoadConfig(StaleProcessingThreshold)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be less than the order stale-processing threshold")
	})

	t.Run("rejects a sub-second rate window that would divide by zero in the limiter", func(t *testing.T) {
		t.Setenv("ORDER_RATE_WINDOW", "500ms")

		_, err := LoadConfig(2 * time.Minute)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ORDER_RATE_WINDOW must be at least 1s")
	})
}
