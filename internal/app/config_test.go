package app_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/app"
	"github.com/residwi/go-api-project-template/internal/config"
)

// The gateway-timeout ceiling spans two modules -- payment's retry budget
// against order's stale-processing threshold -- so neither LoadConfig can see
// both halves and it is asserted here instead.
func TestLoadConfigRejectsAGatewayTimeoutThatOutlivesTheStaleSweep(t *testing.T) {
	settings := &config.Settings{}
	settings.App.Env = "development"

	t.Run("rejects a timeout whose retries outlast the sweep", func(t *testing.T) {
		// PAYMENT_JOB_TIMEOUT must clear payment's own lease-vs-3x-timeout check
		// (3x6m=18m) so this isolates the threshold check instead of tripping both
		// at once. The lease error's text also contains "PAYMENT_GATEWAY_TIMEOUT",
		// so only the threshold wording proves which check fired.
		t.Setenv("JWT_SECRET", "test-secret")
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "6m")
		t.Setenv("PAYMENT_JOB_TIMEOUT", "20m")

		_, err := app.LoadConfig(settings)

		require.Error(t, err)
		require.ErrorContains(t, err, "stale-processing threshold")
	})

	t.Run("accepts a timeout that leaves the sweep room", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "test-secret")
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "10s")

		cfg, err := app.LoadConfig(settings)

		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, cfg.Payment.GatewayTimeout)
	})
}
