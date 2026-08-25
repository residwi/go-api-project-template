package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testContainerPort string

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("testdb")
	defer cleanup()
	testContainerPort = strconv.FormatUint(uint64(pool.Config().ConnConfig.Port), 10)
	os.Exit(m.Run())
}

func TestNewPrimaryPostgres(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		portInt, _ := strconv.Atoi(testContainerPort)
		cfg := config.Database{
			Host: "localhost", Port: portInt,
			User: "test", Password: "test", Name: "testdb", SSLMode: "disable",
			MaxConns: 5, MinConns: 1,
			MaxConnLifetime: 5 * time.Minute, MaxConnIdleTime: 1 * time.Minute,
		}
		pool, err := NewPrimaryPostgres(context.Background(), cfg)
		require.NoError(t, err)
		require.NotNil(t, pool)
		defer pool.Close()
	})

	t.Run("bad ssl mode returns parsing error", func(t *testing.T) {
		portInt, _ := strconv.Atoi(testContainerPort)
		cfg := config.Database{
			Host: "localhost", Port: portInt,
			User: "test", Password: "test", Name: "testdb",
			SSLMode: "invalid-ssl-mode",
		}
		pool, err := NewPrimaryPostgres(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "parsing database config")
	})

	t.Run("connection refused", func(t *testing.T) {
		cfg := config.Database{
			Host: "localhost", Port: 1,
			User: "test", Password: "test", Name: "testdb",
			SSLMode: "disable", MaxConns: 2, MinConns: 1,
		}
		pool, err := NewPrimaryPostgres(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, pool)
	})

	t.Run("zero max conns fails pool creation", func(t *testing.T) {
		portInt, _ := strconv.Atoi(testContainerPort)
		cfg := config.Database{
			Host: "localhost", Port: portInt,
			User: "test", Password: "test", Name: "testdb",
			SSLMode: "disable", MaxConns: 0, MinConns: 0,
		}
		pool, err := NewPrimaryPostgres(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "connecting to database")
	})

	t.Run("ping failure after pool creation", func(t *testing.T) {
		cfg := config.Database{
			Host: "localhost", Port: 1,
			User: "test", Password: "test", Name: "testdb",
			SSLMode: "disable", MaxConns: 2, MinConns: 0,
		}
		pool, err := NewPrimaryPostgres(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging database")
	})
}

func TestNewReplicaPostgres(t *testing.T) {
	t.Run("empty url returns ErrReplicaNotConfigured", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(), config.Database{ReplicaURL: ""})
		require.ErrorIs(t, err, apperror.ErrReplicaNotConfigured)
		assert.Nil(t, pool)
	})

	t.Run("success", func(t *testing.T) {
		dsn := fmt.Sprintf("postgres://test:test@localhost:%s/testdb?sslmode=disable", testContainerPort)
		pool, err := NewReplicaPostgres(context.Background(), config.Database{
			ReplicaURL:      dsn,
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
			config.Database{ReplicaURL: "not-a-valid-url", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "parsing replica database config")
	})

	t.Run("ping failure with unreachable host", func(t *testing.T) {
		pool, err := NewReplicaPostgres(context.Background(),
			config.Database{ReplicaURL: "postgres://x:x@localhost:1/x", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging replica database")
	})
}
