package middleware_test

import (
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testRedis *redis.Client

func TestMain(m *testing.M) {
	rdb, cleanup := testhelper.MustStartRedis(1)
	defer cleanup()
	testRedis = rdb
	os.Exit(m.Run())
}
