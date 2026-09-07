package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedisClient *redis.Client

func TestMain(m *testing.M) {
	client, cleanup := testutil.MustStartRedis(0)
	defer cleanup()
	testRedisClient = client
	os.Exit(m.Run())
}

func TestNewRedis(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		addr := testRedisClient.Options().Addr

		client, err := NewRedis(context.Background(), &redis.Options{Addr: addr})
		require.NoError(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.NoError(t, client.Ping(context.Background()).Err())
	})

	t.Run("connection refused", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		client, err := NewRedis(ctx, &redis.Options{Addr: "localhost:1"})
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "connecting to redis")
	})
}

func TestRedisEmitsCommandSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	client, err := NewRedis(t.Context(), &redis.Options{Addr: testRedisClient.Options().Addr})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, client.Close()) })

	ctx, parent := provider.Tracer("test").Start(t.Context(), "test.Cache")
	require.NoError(t, client.Set(ctx, "trace-key", "value", time.Minute).Err())
	parent.End()

	assert.GreaterOrEqual(t, len(recorder.Ended()), 2)
}
