package category_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// testPool backs this package's service-level integration test
// (service_integration_test.go). It is separate from the postgres
// subpackage's own container/DB to avoid two test binaries racing on the
// same database.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_category_svc")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}
