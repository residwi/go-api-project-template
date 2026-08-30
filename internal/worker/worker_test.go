package worker

import (
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_worker")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestNewClientRejectsARescueWindowThatOutlivesTheStaleThreshold(t *testing.T) {
	_, err := newClient(
		database.DB{Primary: testPool},
		bootstrap.Config{},
		&bootstrap.App{},
		order.StaleProcessingThreshold,
		slog.New(slog.DiscardHandler),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_RESCUE_AFTER")
}
