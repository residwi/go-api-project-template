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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testContainerPort string

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("testdb")
	defer cleanup()
	testContainerPort = strconv.FormatUint(uint64(pool.Config().ConnConfig.Port), 10)
	os.Exit(m.Run())
}

func TestNewPostgres(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		portInt, _ := strconv.Atoi(testContainerPort)
		cfg := config.Database{
			Host: "localhost", Port: portInt,
			User: "test", Password: "test", Name: "testdb", SSLMode: "disable",
			MaxConns: 5, MinConns: 1,
			MaxConnLifetime: 5 * time.Minute, MaxConnIdleTime: 1 * time.Minute,
		}
		pool, err := NewPostgres(context.Background(), cfg)
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
		pool, err := NewPostgres(context.Background(), cfg)
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
		pool, err := NewPostgres(context.Background(), cfg)
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
		pool, err := NewPostgres(context.Background(), cfg)
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
		pool, err := NewPostgres(context.Background(), cfg)
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging database")
	})
}

func TestNewReaderPostgres(t *testing.T) {
	t.Run("empty url returns ErrReaderNotConfigured", func(t *testing.T) {
		pool, err := NewReaderPostgres(context.Background(), config.Database{ReaderURL: ""})
		require.ErrorIs(t, err, apperror.ErrReaderNotConfigured)
		assert.Nil(t, pool)
	})

	t.Run("success", func(t *testing.T) {
		dsn := fmt.Sprintf("postgres://test:test@localhost:%s/testdb?sslmode=disable", testContainerPort)
		pool, err := NewReaderPostgres(context.Background(), config.Database{
			ReaderURL:       dsn,
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
		pool, err := NewReaderPostgres(context.Background(),
			config.Database{ReaderURL: "not-a-valid-url", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "parsing reader database config")
	})

	t.Run("ping failure with unreachable host", func(t *testing.T) {
		pool, err := NewReaderPostgres(context.Background(),
			config.Database{ReaderURL: "postgres://x:x@localhost:1/x", MaxConns: 5})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "pinging reader database")
	})
}
