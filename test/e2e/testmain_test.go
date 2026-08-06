// Package e2e_test owns Postgres database "test_e2e" and Redis DB index 5, both
// exclusively:
// cleanup drops the database WITH (FORCE) and flushes the index, so sharing
// either would tear down another package's fixtures mid-run. The registry is in
// internal/testhelper.
package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/config"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testDeps  *apihttp.Deps
	testApp   *bootstrap.App
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testhelper.MustStartPostgres("test_e2e")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testhelper.MustStartRedis(5)
	defer cleanupRedis()
	testRedis = rdb

	testDeps = &apihttp.Deps{
		Config: &config.Config{
			App: config.AppConfig{
				Name:         "test",
				Env:          "development",
				Port:         8080,
				MaxCartItems: 50,
			},
			JWT: config.JWTConfig{
				Secret:          "test-secret-key-at-least-32-chars-long",
				AccessTokenTTL:  15 * time.Minute,
				RefreshTokenTTL: 168 * time.Hour,
				Issuer:          "test",
			},
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
				AllowedHeaders: []string{"Content-Type", "Authorization"},
				MaxAge:         86400,
			},
			Payment: config.PaymentConfig{
				Gateway:        "mock",
				GatewayURL:     "http://localhost:19999",
				GatewayTimeout: 5 * time.Second,
			},
		},
		Pool:   pool,
		Cache:  rdb,
		Logger: testhelper.DiscardLogger(),
	}

	testApp = newTestApp(testDeps.Config)

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
	testhelper.ResetRedis(t, testRedis)
}

// newTestApp wires a bootstrap.App against testPool/testRedis for a given
// Config. TestMain uses it for testApp; tests that need a different payment
// gateway URL (a local httptest mock server) build their own Config and call
// this instead.
//
// internal/transport/http/router_test.go carries its own copy. Keep them in step.
func newTestApp(cfg *config.Config) *bootstrap.App {
	app, err := bootstrap.New(bootstrap.Deps{
		Config: cfg,
		Pool:   testPool,
		Cache:  testRedis,
		Logger: testhelper.DiscardLogger(),
	})
	if err != nil {
		panic(err)
	}
	return app
}

// newPaymentService wires a whole App against a custom gateway URL (a local
// httptest mock server) and hands back just the payment service, so a saga
// test can drive a job directly. SetOrderPaymentDeps, inside bootstrap.New,
// closes the order/payment cycle, as NewRouter does.
//
// internal/transport/http/router_test.go carries its own copy. Keep them in step.
func newPaymentService(t *testing.T, gatewayURL string) *payment.Service {
	t.Helper()

	return newTestApp(&config.Config{
		App:  testDeps.Config.App,
		JWT:  testDeps.Config.JWT,
		CORS: testDeps.Config.CORS,
		Payment: config.PaymentConfig{
			Gateway:        "mock",
			GatewayURL:     gatewayURL,
			GatewayTimeout: 5 * time.Second,
		},
	}).Payments
}

// ReserveBatch and DeductBatch need a row to update, and these flows insert
// products with raw SQL, bypassing the EnsureLevel in product.Service.Create.
//
// internal/transport/http/router_test.go carries its own copy. Keep them in step.
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
