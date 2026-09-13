package config

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabase_DSN(t *testing.T) {
	t.Run("returns correctly formatted DSN with all fields", func(t *testing.T) {
		cfg := Database{
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
		cfg := Database{
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

func TestRedis_Addr(t *testing.T) {
	t.Run("returns host:port format", func(t *testing.T) {
		cfg := Redis{
			Host: "redis.example.com",
			Port: 6380,
		}

		assert.Equal(t, "redis.example.com:6380", cfg.Addr())
	})
}

func TestLoad(t *testing.T) {
	// No t.Parallel below: t.Setenv panics in a parallel test.
	t.Run("parses log settings so a logger can exist before module config loads", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "warn")
		t.Setenv("LOG_FORMAT", "text")

		appConfig, err := Load()

		require.NoError(t, err)
		assert.Equal(t, Log{Level: "warn", Format: "text"}, appConfig.Log)
	})

	t.Run("rejects a zero APP_SHUTDOWN_TIMEOUT", func(t *testing.T) {
		t.Setenv("APP_SHUTDOWN_TIMEOUT", "0")

		_, err := Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "APP_SHUTDOWN_TIMEOUT")
	})
}

func TestParseTrustedProxies(t *testing.T) {
	t.Run("parses IPv4 and IPv6 CIDRs", func(t *testing.T) {
		got, err := ParseTrustedProxies([]string{"203.0.113.0/24", "2001:db8::/32"})

		require.NoError(t, err)
		assert.Equal(t, []netip.Prefix{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("2001:db8::/32"),
		}, got)
	})

	t.Run("returns nothing for an unset list", func(t *testing.T) {
		got, err := ParseTrustedProxies(nil)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("skips blank entries", func(t *testing.T) {
		got, err := ParseTrustedProxies([]string{"", "   ", "203.0.113.0/24"})

		require.NoError(t, err)
		assert.Equal(t, []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}, got)
	})

	t.Run("rejects a malformed entry", func(t *testing.T) {
		_, err := ParseTrustedProxies([]string{"203.0.113.0/24", "not-a-cidr"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not-a-cidr")
	})

	t.Run("rejects a bare address with no prefix length", func(t *testing.T) {
		_, err := ParseTrustedProxies([]string{"203.0.113.5"})

		require.Error(t, err)
	})
}

func TestSettingsValidateTrustedProxies(t *testing.T) {
	t.Run("aborts boot on a malformed CIDR", func(t *testing.T) {
		var s Settings
		s.App.ShutdownTimeout = 30 * time.Second
		s.App.TrustedProxies = []string{"nonsense"}

		err := s.validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXIES")
	})

	t.Run("accepts a valid CIDR", func(t *testing.T) {
		var s Settings
		s.App.ShutdownTimeout = 30 * time.Second
		s.App.TrustedProxies = []string{"203.0.113.0/24"}

		assert.NoError(t, s.validate())
	})
}
