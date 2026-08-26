// Package e2e_test owns Postgres database "test_e2e" and Redis DB index 5, both
// exclusively:
// cleanup drops the database WITH (FORCE) and flushes the index, so sharing
// either would tear down another package's fixtures mid-run. The registry is in
// internal/testutil.
package e2e_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/server"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testApp   *bootstrap.App

	// Fixed across every App this package builds, so a token minted by one is
	// still valid against another -- the only config any e2e test actually
	// varies is Payment's gateway URL, pointed at a local httptest mock server.
	testInfra = &config.Settings{
		App: config.App{
			Name: "test",
			Env:  "development",
			Port: 8080,
		},
		CORS: config.CORS{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         86400,
		},
	}
	testAuthCfg = auth.Config{
		Secret:          "test-secret-key-at-least-32-chars-long",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 168 * time.Hour,
		Issuer:          "test",
	}
	testCartCfg    = cart.Config{MaxItems: 50}
	testOrderCfg   order.Config
	testPaymentCfg = payment.Config{
		Gateway:        "mock",
		GatewayURL:     "http://localhost:19999",
		GatewayTimeout: 5 * time.Second,
	}
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testutil.MustStartPostgres("test_e2e")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testutil.MustStartRedis(5)
	defer cleanupRedis()
	testRedis = rdb

	testApp = newTestApp(testPaymentCfg)

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testutil.ResetDB(t, testPool)
	testutil.ResetRedis(t, testRedis)
}

// newTestApp wires a bootstrap.App against testPool/testRedis for a given
// payment config. TestMain uses it for testApp; tests that need a different
// payment gateway URL (a local httptest mock server) build their own
// payment.Config and call this instead.
//
// internal/server/router_test.go carries its own copy. Keep them in step.
func newTestApp(paymentCfg payment.Config) *bootstrap.App {
	app, err := bootstrap.New(
		testAuthCfg,
		testCartCfg,
		paymentCfg,
		database.DB{Primary: testPool},
		testRedis,
		testutil.DiscardLogger(),
	)
	if err != nil {
		panic(err)
	}
	return app
}

// newTestRouter builds a router for a given payment config against a
// freshly wired app -- the common case, where nothing but Payment varies
// between call sites.
//
// internal/server/router_test.go carries its own copy. Keep them in step.
func newTestRouter(paymentCfg payment.Config) http.Handler {
	return server.NewRouter(
		testInfra, testAuthCfg, testOrderCfg, paymentCfg,
		testRedis, testutil.DiscardLogger(),
		newTestApp(paymentCfg),
	)
}

// newPaymentService wires a whole App against a custom gateway URL (a local
// httptest mock server) and hands back the payment service, so a saga test
// can drive a job directly through its NewRefundJob's Run.
//
// internal/server/router_test.go carries its own copy. Keep them in step.
func newPaymentService(t *testing.T, gatewayURL string) *payment.Service {
	t.Helper()

	return newTestApp(payment.Config{
		Gateway:        "mock",
		GatewayURL:     gatewayURL,
		GatewayTimeout: 5 * time.Second,
	}).Payments
}

// Reserve and Deduct need a row to update, and these flows insert
// products with raw SQL, bypassing the EnsureLevel in product.Service.Create.
//
// internal/server/router_test.go carries its own copy. Keep them in step.
func seedInventoryLevel(t *testing.T, productID uuid.UUID, available, reserved int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO inventory_levels (product_id, available_stock, reserved_stock)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (product_id) DO UPDATE
		 SET available_stock = EXCLUDED.available_stock,
		     reserved_stock  = EXCLUDED.reserved_stock`,
		productID, available, reserved)
	require.NoError(t, err)
}

func inventoryLevelOf(t *testing.T, productID uuid.UUID) (available, reserved int) {
	t.Helper()
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	return available, reserved
}
