package user_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/user/postgres"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// testPool and testRedis back this package's service-level integration test
// (service_integration_test.go). They are separate from the postgres
// subpackage's own containers/DB/Redis index to avoid two test binaries
// racing on the same database or Redis keyspace.
var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testhelper.MustStartPostgres("test_features_user_svc")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testhelper.MustStartRedis(4)
	defer cleanupRedis()
	testRedis = rdb

	os.Exit(m.Run())
}

func seedUser(t *testing.T) *user.User {
	t.Helper()
	id := testhelper.SeedUser(t, testPool)

	repo := postgres.New(testPool)
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}
