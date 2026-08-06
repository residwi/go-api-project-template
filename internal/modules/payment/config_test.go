package payment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: every subtest calls t.Setenv, which panics if the test (or an
// ancestor) has called t.Parallel. This file is on the paralleltest exclusion
// list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("requires a real webhook secret outside development", func(t *testing.T) {
		t.Setenv("PAYMENT_WEBHOOK_SECRET", defaultWebhookSecret)

		_, err := LoadConfig("production", 2*time.Minute)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_WEBHOOK_SECRET must be set")
	})

	t.Run("allows the placeholder secret in development", func(t *testing.T) {
		t.Setenv("PAYMENT_WEBHOOK_SECRET", defaultWebhookSecret)

		_, err := LoadConfig("development", 2*time.Minute)

		require.NoError(t, err)
	})

	t.Run("rejects a lease shorter than three gateway timeouts, which would double-charge", func(t *testing.T) {
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "1m")

		_, err := LoadConfig("development", 2*time.Minute)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "WORKER_LEASE_DURATION must be at least 3")
	})

	t.Run("rejects a gateway timeout so large no valid lease exists", func(t *testing.T) {
		// jobsLease must clear the lease-vs-3x-timeout check on its own (3x6m=18m)
		// so this isolates the threshold check instead of tripping both at once --
		// the brief's own 5m draft trips both, and the lease error's text also
		// contains "PAYMENT_GATEWAY_TIMEOUT", so it would pass for the wrong reason.
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "6m")

		_, err := LoadConfig("development", 20*time.Minute)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_GATEWAY_TIMEOUT")
	})
}
