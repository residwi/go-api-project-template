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

func TestTrustedProxiesDecode(t *testing.T) {
	t.Run("parses IPv4 and IPv6 CIDRs", func(t *testing.T) {
		var got TrustedProxies

		require.NoError(t, got.Decode("203.0.113.0/24,2001:db8::/32"))
		assert.Equal(t, TrustedProxies{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("2001:db8::/32"),
		}, got)
	})

	t.Run("returns nothing for an empty value", func(t *testing.T) {
		var got TrustedProxies

		require.NoError(t, got.Decode(""))
		assert.Empty(t, got)
	})

	t.Run("skips blank entries and trims the rest", func(t *testing.T) {
		var got TrustedProxies

		require.NoError(t, got.Decode(" , 203.0.113.0/24 ,   "))
		assert.Equal(t, TrustedProxies{netip.MustParsePrefix("203.0.113.0/24")}, got)
	})

	t.Run("rejects a malformed entry", func(t *testing.T) {
		var got TrustedProxies

		err := got.Decode("203.0.113.0/24,not-a-cidr")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not-a-cidr")
	})

	t.Run("rejects a bare address with no prefix length", func(t *testing.T) {
		var got TrustedProxies

		require.Error(t, got.Decode("203.0.113.5"))
	})

	t.Run("replaces any earlier value rather than appending to it", func(t *testing.T) {
		got := TrustedProxies{netip.MustParsePrefix("198.51.100.0/24")}

		require.NoError(t, got.Decode("203.0.113.0/24"))
		assert.Equal(t, TrustedProxies{netip.MustParsePrefix("203.0.113.0/24")}, got)
	})
}

func TestLoadTrustedProxies(t *testing.T) {
	t.Run("aborts boot on a malformed CIDR", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "nonsense")

		_, err := Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonsense")
	})

	t.Run("parses a valid list onto App.TrustedProxies", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "203.0.113.0/24,2001:db8::/32")

		s, err := Load()

		require.NoError(t, err)
		assert.Equal(t, TrustedProxies{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("2001:db8::/32"),
		}, s.App.TrustedProxies)
	})
}
