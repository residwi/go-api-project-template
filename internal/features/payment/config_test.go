package payment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No t.Parallel: every subtest calls t.Setenv, which panics if the test (or an
// ancestor) has called t.Parallel. This file is on the paralleltest exclusion
// list in .golangci.yml for that reason.
func TestLoadConfig(t *testing.T) {
	t.Run("rejects an unrecognised gateway", func(t *testing.T) {
		// A typo, a stray capital or trailing space must abort boot: newPaymentGateway
		// (module.go) falls back to the mock for anything it does not
		// recognise, which would otherwise route every real charge at
		// localhost silently.
		t.Setenv("PAYMENT_GATEWAY", "Stripe")

		_, err := LoadConfig("development")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_GATEWAY must be")
	})

	t.Run("accepts each gateway newPaymentGateway knows how to build", func(t *testing.T) {
		for _, name := range []string{GatewayMock, GatewayStripe, GatewayMidtrans} {
			t.Setenv("PAYMENT_GATEWAY", name)

			_, err := LoadConfig("development")

			require.NoError(t, err, "gateway %q must be accepted", name)
		}
	})

	t.Run("requires a real webhook secret outside development", func(t *testing.T) {
		t.Setenv("PAYMENT_WEBHOOK_SECRET", defaultWebhookSecret)

		_, err := LoadConfig("production")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_WEBHOOK_SECRET must be set")
	})

	t.Run("allows the placeholder secret in development", func(t *testing.T) {
		t.Setenv("PAYMENT_WEBHOOK_SECRET", defaultWebhookSecret)

		_, err := LoadConfig("development")

		require.NoError(t, err)
	})

	t.Run("rejects a job interval that would hammer the database", func(t *testing.T) {
		t.Setenv("PAYMENT_JOB_INTERVAL", "1s")

		_, err := LoadConfig("development")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_JOB_INTERVAL must be at least 5s")
	})

	t.Run("rejects a zero job concurrency that would deadlock the runner", func(t *testing.T) {
		t.Setenv("PAYMENT_JOB_CONCURRENCY", "0")

		_, err := LoadConfig("development")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_JOB_CONCURRENCY must be at least 1")
	})

	t.Run("rejects a lease shorter than three gateway timeouts, which would double-charge", func(t *testing.T) {
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "1m")

		_, err := LoadConfig("development")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_JOB_TIMEOUT must be at least 3")
	})

	t.Run("rejects a gateway timeout so large no valid lease exists", func(t *testing.T) {
		// PAYMENT_JOB_TIMEOUT must clear the lease-vs-3x-timeout check on its own
		// (3x6m=18m) so this isolates the threshold check instead of tripping both
		// at once -- the brief's own 5m draft trips both, and the lease error's
		// text also contains "PAYMENT_GATEWAY_TIMEOUT", so it would pass for the
		// wrong reason.
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "6m")
		t.Setenv("PAYMENT_JOB_TIMEOUT", "20m")

		_, err := LoadConfig("development")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_GATEWAY_TIMEOUT")
	})
}
