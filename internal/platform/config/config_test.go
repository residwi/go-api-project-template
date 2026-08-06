package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConfig_DSN(t *testing.T) {
	t.Run("returns correctly formatted DSN with all fields", func(t *testing.T) {
		cfg := DatabaseConfig{
			Host:                            "db.example.com",
			Port:                            5432,
			User:                            "admin",
			Password:                        "secret",
			Name:                            "shop",
			SSLMode:                         "require",
			StatementTimeout:                30 * time.Second,
			IdleInTransactionSessionTimeout: 60 * time.Second,
		}

		expected := "postgres://admin:secret@db.example.com:5432/shop?sslmode=require&statement_timeout=30000&idle_in_transaction_session_timeout=60000"
		assert.Equal(t, expected, cfg.DSN())
	})

	t.Run("includes statement_timeout and idle_in_tx_session_timeout in milliseconds", func(t *testing.T) {
		cfg := DatabaseConfig{
			Host:                            "localhost",
			Port:                            5432,
			User:                            "postgres",
			Password:                        "postgres",
			Name:                            "testdb",
			SSLMode:                         "disable",
			StatementTimeout:                15 * time.Second,
			IdleInTransactionSessionTimeout: 45 * time.Second,
		}

		dsn := cfg.DSN()
		assert.Contains(t, dsn, "statement_timeout=15000")
		assert.Contains(t, dsn, "idle_in_transaction_session_timeout=45000")
	})
}

func TestRedisConfig_Addr(t *testing.T) {
	t.Run("returns host:port format", func(t *testing.T) {
		cfg := RedisConfig{
			Host: "redis.example.com",
			Port: 6380,
		}

		assert.Equal(t, "redis.example.com:6380", cfg.Addr())
	})
}

func TestLoadInfra(t *testing.T) {
	// No t.Parallel below: t.Setenv panics in a parallel test.
	t.Run("parses log settings so a logger can exist before module config loads", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "warn")
		t.Setenv("LOG_FORMAT", "text")

		infra, err := LoadInfra()

		require.NoError(t, err)
		assert.Equal(t, LogConfig{Level: "warn", Format: "text"}, infra.Log)
	})

	t.Run("rejects a worker interval that would hammer the database", func(t *testing.T) {
		t.Setenv("WORKER_INTERVAL", "1s")

		_, err := LoadInfra()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "WORKER_INTERVAL must be at least 5s")
	})

	t.Run("rejects a zero worker concurrency that would deadlock the runner", func(t *testing.T) {
		t.Setenv("WORKER_CONCURRENCY", "0")

		_, err := LoadInfra()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "WORKER_CONCURRENCY must be at least 1")
	})
}
