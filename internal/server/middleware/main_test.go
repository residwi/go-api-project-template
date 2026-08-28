package middleware

import (
	"log/slog"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedis *redis.Client

func TestMain(m *testing.M) {
	rdb, cleanup := testutil.MustStartRedis(1)
	defer cleanup()
	testRedis = rdb
	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
