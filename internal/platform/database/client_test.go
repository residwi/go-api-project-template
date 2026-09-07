package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

var (
	testContainerPort string
	testPool          *pgxpool.Pool
)

func TestMain(m *testing.M) {
	var cleanup func()
	testPool, cleanup = testutil.MustStartPostgres("testdb")
	defer cleanup()
	testContainerPort = strconv.FormatUint(uint64(testPool.Config().ConnConfig.Port), 10)
	os.Exit(m.Run())
}

func TestNewPrimaryPostgres(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		opts := PostgresOptions{
			DSN:             testDSN(testContainerPort, "disable"),
			MaxConns:        5,
			MinConns:        1,
			MaxConnLifetime: 5 * time.Minute,
			MaxConnIdleTime: 1 * time.Minute,
		}
		pool, err := NewPrimaryPostgres(context.Background(), opts)
		require.NoError(t, err)
		require.NotNil(t, pool)
		defer pool.Close()
	})

	t.Run("bad ssl mode returns parsing error", func(t *testing.T) {
		opts := PostgresOptions{DSN: testDSN(testContainerPort, "invalid-ssl-mode")}
		pool, err := NewPrimaryPostgres(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "parsing database config")
	})

	t.Run("connection refused", func(t *testing.T) {
		opts := PostgresOptions{DSN: testDSN("1", "disable"), MaxConns: 2, MinConns: 1}
		pool, err := NewPrimaryPostgres(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, pool)
	})

	t.Run("zero max conns fails pool creation", func(t *testing.T) {
		opts := PostgresOptions{DSN: testDSN(testContainerPort, "disable"), MaxConns: 0, MinConns: 0}
		pool, err := NewPrimaryPostgres(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "connecting to database")
	})

	t.Run("ping failure after pool creation", func(t *testing.T) {
		opts := PostgresOptions{DSN: testDSN("1", "disable"), MaxConns: 2, MinConns: 0}
		pool, err := NewPrimaryPostgres(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging database")
	})
}

func TestNewReplicaPostgres(t *testing.T) {
	t.Run("empty dsn returns ErrReplicaNotConfigured", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(), PostgresOptions{DSN: ""})
		require.ErrorIs(t, err, ErrReplicaNotConfigured)
		assert.Nil(t, pool)
	})

	t.Run("success", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(), PostgresOptions{
			DSN:             testDSN(testContainerPort, "disable"),
			MaxConns:        5,
			MinConns:        1,
			MaxConnLifetime: 5 * time.Minute,
			MaxConnIdleTime: 1 * time.Minute,
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
		defer pool.Close()
	})

	t.Run("invalid dsn returns parsing error", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(),
			PostgresOptions{DSN: "not-a-valid-url", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "parsing replica database config")
	})

	t.Run("ping failure with unreachable host", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(),
			PostgresOptions{DSN: "postgres://x:x@localhost:1/x", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging replica database")
	})
}

func TestPoolEmitsQuerySpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	opts := PostgresOptions{
		DSN:             testDSN(testContainerPort, "disable"),
		MaxConns:        5,
		MinConns:        1,
		MaxConnLifetime: 5 * time.Minute,
		MaxConnIdleTime: 1 * time.Minute,
	}
	pool, err := NewPrimaryPostgres(context.Background(), opts)
	require.NoError(t, err)
	defer pool.Close()

	ctx, parent := provider.Tracer("test").Start(t.Context(), "test.Query")
	var one int
	require.NoError(t, pool.QueryRow(ctx, "SELECT 1").Scan(&one))
	parent.End()

	names := make([]string, 0, len(recorder.Ended()))
	for _, span := range recorder.Ended() {
		names = append(names, span.Name())
	}

	assert.Contains(t, names, "query SELECT")
}

func testDSN(port, sslMode string) string {
	return fmt.Sprintf("postgres://test:test@localhost:%s/testdb?sslmode=%s", port, sslMode)
}
