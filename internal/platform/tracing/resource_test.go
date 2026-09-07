package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestNewResource(t *testing.T) {
	t.Run("uses the given service name when the environment sets none", func(t *testing.T) {
		res, err := newResource(t.Context(), "ecommerce-api", "development")
		require.NoError(t, err)

		assert.Contains(t, res.Attributes(), attribute.String("service.name", "ecommerce-api"))
		assert.Contains(t, res.Attributes(), attribute.String("deployment.environment.name", "development"))
	})

	t.Run("lets OTEL_SERVICE_NAME override the given service name", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "checkout-api")

		res, err := newResource(t.Context(), "ecommerce-api", "development")
		require.NoError(t, err)

		assert.Contains(t, res.Attributes(), attribute.String("service.name", "checkout-api"))
	})
}
