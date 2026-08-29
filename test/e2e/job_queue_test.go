package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/server"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

type jobQueueEnv struct {
	app     *bootstrap.App
	handler http.Handler
}

func newTestEnv(t *testing.T) jobQueueEnv {
	t.Helper()

	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testutil.DiscardLogger())
	mockServer := httptest.NewServer(mockMux)
	t.Cleanup(mockServer.Close)

	paymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 5 * time.Second,
	}
	app := newTestApp(paymentCfg)
	handler := server.NewRouter(
		testAppCfg, withPayment(paymentCfg),
		testRedis, testutil.DiscardLogger(),
		app,
	)
	return jobQueueEnv{app: app, handler: handler}
}

func placeAndPayOrder(t *testing.T, env jobQueueEnv) (orderID, paymentID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'JobQueue Cat', $2, true)`,
		catID, "jobqueue-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'JobQueue Product', $2, 'desc', 4000, 'USD', 'published', $3)`,
		prodID, "jobqueue-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	email := "jobqueue-" + uuid.New().String()[:8] + "@example.com"
	userID, token := registerE2EUser(t, env.handler, email)
	t.Cleanup(func() { cleanupOrdersOf(userID) })

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	env.handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	orderReq := httptest.NewRequest(http.MethodPost, "/api/orders",
		strings.NewReader(`{"payment_method_id":"pm_test_jobqueue"}`))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	orderReq.Header.Set("Idempotency-Key", uuid.New().String())
	orderW := httptest.NewRecorder()
	env.handler.ServeHTTP(orderW, orderReq)
	require.Equal(t, http.StatusCreated, orderW.Code)

	var orderResp map[string]any
	require.NoError(t, json.NewDecoder(orderW.Body).Decode(&orderResp))
	orderID = uuid.MustParse(orderResp["data"].(map[string]any)["order"].(map[string]any)["id"].(string))

	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM payments WHERE order_id = $1`, orderID).Scan(&paymentID))
	require.Equal(t, "paid", orderStatusOf(t, orderID))

	return orderID, paymentID
}

func TestRefundJobIsEnqueuedOnce(t *testing.T) {
	setup(t)
	ctx := context.Background()
	env := newTestEnv(t)

	_, paymentID := placeAndPayOrder(t, env)

	require.NoError(t, env.app.Payments.Refund(ctx, paymentID))
	require.NoError(t, env.app.Payments.Refund(ctx, paymentID))

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM river_job WHERE kind = 'payment.refund' AND args->>'PaymentID' = $1`,
		paymentID.String()).Scan(&count))

	assert.Equal(t, 1, count, "River's per-args uniqueness must prevent a second refund job for the same payment")
}

func TestCancellingAnOrderCancelsItsPaymentJobsOnly(t *testing.T) {
	setup(t)
	ctx := context.Background()
	env := newTestEnv(t)

	orderID, paymentID := placeAndPayOrder(t, env)
	require.NoError(t, env.app.Payments.Refund(ctx, paymentID))

	_, err := testPool.Exec(ctx,
		`INSERT INTO river_job (kind, queue, max_attempts, args, tags)
		 VALUES ('test.foreign', 'default', 1, '{}', ARRAY[$1])`,
		"order-"+orderID.String())
	require.NoError(t, err)

	require.NoError(t, env.app.Payments.CancelPendingByOrderID(ctx, orderID))

	var jobState string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT state FROM river_job WHERE kind = 'payment.refund' AND args->>'PaymentID' = $1`,
		paymentID.String()).Scan(&jobState))
	assert.Equal(t, "cancelled", jobState)

	var foreignJobState string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT state FROM river_job WHERE kind = 'test.foreign' AND $1 = ANY(tags)`,
		"order-"+orderID.String()).Scan(&foreignJobState))
	assert.Equal(t, "available", foreignJobState,
		"CancelPendingByOrderID must only cancel payment's own kind, not every job tagged for this order")

	var sendPending int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM river_job WHERE kind = 'notification.send' AND state = 'available'`).Scan(&sendPending))
	assert.Positive(t, sendPending)
}
