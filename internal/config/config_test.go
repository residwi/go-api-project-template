package config

import (
	"os"
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

func TestConfig_Validate(t *testing.T) {
	t.Run("error when WebhookSecret is default in non-dev environment", func(t *testing.T) {
		cfg := Config{
			App:     AppConfig{Env: "production"},
			Payment: PaymentConfig{WebhookSecret: "webhook-secret", GatewayTimeout: 10 * time.Second},
			Worker:  WorkerConfig{LeaseDuration: 2 * time.Minute, Interval: 10 * time.Second},
		}

		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_WEBHOOK_SECRET must be set in non-development environments")
	})

	t.Run("error when LeaseDuration less than 3x GatewayTimeout", func(t *testing.T) {
		cfg := Config{
			App:     AppConfig{Env: "production"},
			Payment: PaymentConfig{WebhookSecret: "real-secret", GatewayTimeout: 10 * time.Second},
			Worker:  WorkerConfig{LeaseDuration: 20 * time.Second, Interval: 10 * time.Second},
		}

		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WORKER_LEASE_DURATION must be at least 3×")
	})

	t.Run("error when WorkerInterval less than 5s", func(t *testing.T) {
		cfg := Config{
			App:     AppConfig{Env: "production"},
			Payment: PaymentConfig{WebhookSecret: "real-secret", GatewayTimeout: 10 * time.Second},
			Worker:  WorkerConfig{LeaseDuration: 2 * time.Minute, Interval: 3 * time.Second},
		}

		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WORKER_INTERVAL must be at least 5s")
	})

	t.Run("passes with valid production config", func(t *testing.T) {
		cfg := Config{
			App:     AppConfig{Env: "production"},
			Payment: PaymentConfig{WebhookSecret: "real-secret", GatewayTimeout: 10 * time.Second},
			Worker:  WorkerConfig{LeaseDuration: 2 * time.Minute, Interval: 10 * time.Second},
		}

		err := cfg.validate()
		assert.NoError(t, err)
	})

	t.Run("passes in development with default webhook secret", func(t *testing.T) {
		cfg := Config{
			App:     AppConfig{Env: "development"},
			Payment: PaymentConfig{WebhookSecret: "webhook-secret", GatewayTimeout: 10 * time.Second},
			Worker:  WorkerConfig{LeaseDuration: 2 * time.Minute, Interval: 10 * time.Second},
		}

		err := cfg.validate()
		assert.NoError(t, err)
	})
}

func TestLoad(t *testing.T) {
	t.Run("success with valid env vars", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "test-jwt-secret")
		t.Setenv("APP_ENV", "development")
		t.Setenv("WORKER_INTERVAL", "10s")
		t.Setenv("WORKER_LEASE_DURATION", "2m")
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "10s")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "test-jwt-secret", cfg.JWT.Secret)
		assert.Equal(t, "development", cfg.App.Env)
	})

	t.Run("missing required JWT_SECRET", func(t *testing.T) {
		// envconfig considers an env var "missing" only when it's truly unset.
		// t.Setenv records the current value and will restore it after the test.
		t.Setenv("JWT_SECRET", "placeholder")
		os.Unsetenv("JWT_SECRET")

		cfg, err := Load()
		assert.Nil(t, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading config")
	})

	t.Run("validation error propagates", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "test-jwt-secret")
		t.Setenv("APP_ENV", "production")
		t.Setenv("PAYMENT_WEBHOOK_SECRET", "webhook-secret")
		t.Setenv("WORKER_INTERVAL", "10s")
		t.Setenv("WORKER_LEASE_DURATION", "2m")
		t.Setenv("PAYMENT_GATEWAY_TIMEOUT", "10s")

		cfg, err := Load()
		assert.Nil(t, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PAYMENT_WEBHOOK_SECRET")
	})
}
