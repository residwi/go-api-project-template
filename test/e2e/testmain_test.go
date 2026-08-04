// Package e2e_test drives whole sagas -- cart to order to charge to webhook to
// refund -- through the real HTTP router, a real Postgres and, for payment
// flows, the real mock payment gateway served over a local test HTTP server.
//
// These tests used to sit in internal/transport/http/router_test.go, where they
// made that file's name lie about its contents and put the suite's slowest work
// inside a unit-test package. router_test.go now covers routing, middleware,
// CORS, health and server lifecycle; the sagas live here.
//
// This package owns Postgres database "test_e2e" and Redis DB index 5. Both are
// exclusive: testhelper drops the database WITH (FORCE) and flushes the Redis
// index on cleanup, so sharing either with another package would tear down that
// package's fixtures mid-run. The index registry is in internal/testhelper.
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
	"github.com/residwi/go-api-project-template/internal/config"
	cartpg "github.com/residwi/go-api-project-template/internal/modules/cart/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	notificationpg "github.com/residwi/go-api-project-template/internal/modules/notification/postgres"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	mockgateway "github.com/residwi/go-api-project-template/internal/modules/payment/mock"
	paymentpg "github.com/residwi/go-api-project-template/internal/modules/payment/postgres"
	productpg "github.com/residwi/go-api-project-template/internal/modules/product/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	promotionpg "github.com/residwi/go-api-project-template/internal/modules/promotion/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
	testDeps  *apihttp.Deps
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

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
	testhelper.ResetRedis(t, testRedis)
}

// newPaymentService composes a payment service the way cmd/worker does, so a
// saga test can drive a job directly. gatewayURL points at the test's mock
// gateway server. The order⇄payment cycle is closed by SetOrderPaymentDeps,
// exactly as NewRouter does it.
func newPaymentService(t *testing.T, gatewayURL string) *payment.Service {
	t.Helper()

	txRunner := database.NewTxRunner(testPool)
	inventorySvc := inventory.NewService(inventorypg.New(testPool))
	productSvc := bootstrap.NewProductService(productpg.New(testPool), inventorySvc)
	cartSvc := bootstrap.NewCartService(cartpg.New(testPool), txRunner, productSvc, testDeps.Config.App.MaxCartItems)
	promotionSvc := promotion.NewService(promotionpg.New(testPool), txRunner)
	notificationSvc := notification.NewService(notificationpg.New(testPool), testhelper.DiscardLogger())

	orderSvc := bootstrap.NewOrderService(
		orderpg.New(testPool), txRunner, cartSvc, inventorySvc, promotionSvc, notificationSvc,
		testhelper.DiscardLogger(),
	)
	gw := mockgateway.New(gatewayURL, 5*time.Second)
	paymentSvc := bootstrap.NewPaymentService(
		paymentpg.New(testPool), txRunner, gw, orderSvc, inventorySvc, promotionSvc,
		testhelper.DiscardLogger(),
	)
	bootstrap.SetOrderPaymentDeps(orderSvc, paymentSvc)

	return paymentSvc
}

// seedInventoryLevel gives a product an inventory_levels row so ReserveBatch/
// DeductBatch have something to update. product.Service.Create does register a
// new product with inventory (via EnsureLevel), but the row it writes is zeroed,
// and these flows insert products with raw SQL anyway, bypassing Create
// entirely -- so there is no row at all. Either way the stock is seeded here.
//
// internal/transport/http/router_test.go carries its own copy for
// TestAdapterErrorPaths_PaymentJobWithDeletedOrder. Keep the two in step.
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

// inventoryLevelOf reads back a product's current available/reserved split.
// Checkout and refund flows mutate inventory_levels, not products.
func inventoryLevelOf(t *testing.T, productID uuid.UUID) (available, reserved int) {
	t.Helper()
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT available_stock, reserved_stock FROM inventory_levels WHERE product_id = $1`,
		productID).Scan(&available, &reserved))
	return available, reserved
}
