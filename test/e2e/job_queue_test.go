package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
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
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
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
	deps := &server.Deps{
		Infra:   testDeps.Infra,
		Auth:    testDeps.Auth,
		Order:   testDeps.Order,
		Payment: paymentCfg,
		DB:      database.DB{Primary: testPool},
		Cache:   testRedis,
		Logger:  testutil.DiscardLogger(),
	}
	return jobQueueEnv{app: app, handler: server.NewRouter(deps, app)}
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

	orderID, paymentID := placeAndPayOrder(t, env)

	require.NoError(t, env.app.Payments.Refund(ctx, paymentID))
	require.Error(t, env.app.Payments.Refund(ctx, paymentID))

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_queue WHERE dedup_key = $1 AND status IN ('pending', 'processing')`,
		"payment.refund:"+paymentID.String()).Scan(&count))

	assert.Equal(t, 1, count)
	_ = orderID
}

func TestCancellingAnOrderCancelsItsPaymentJobsOnly(t *testing.T) {
	setup(t)
	ctx := context.Background()
	env := newTestEnv(t)

	orderID, paymentID := placeAndPayOrder(t, env)
	require.NoError(t, env.app.Payments.Refund(ctx, paymentID))

	require.NoError(t, env.app.Payments.CancelPendingByOrderID(ctx, orderID))

	var paymentStatus string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT status FROM job_queue WHERE dedup_key = $1`,
		"payment.refund:"+paymentID.String()).Scan(&paymentStatus))
	assert.Equal(t, "cancelled", paymentStatus)

	var sendPending int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_queue WHERE queue = 'notification' AND status = 'pending'`).Scan(&sendPending))
	assert.Positive(t, sendPending)
}

func TestRunnerClaimsAndRunsAnEnqueuedJob(t *testing.T) {
	setup(t)
	ctx := context.Background()
	env := newTestEnv(t)

	orderID, paymentID := placeAndPayOrder(t, env)

	require.NoError(t, jobs.Enqueue(ctx, env.app.JobStore, payment.RefundJob{
		PaymentID: paymentID,
		OrderID:   orderID,
	}, jobs.Keys{Dedup: "runner-claims:" + paymentID.String()}))

	var queue, status string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT queue, status FROM job_queue WHERE dedup_key = $1`,
		"runner-claims:"+paymentID.String(),
	).Scan(&queue, &status))
	require.Equal(t, "payment", queue, "kind payment.refund must land on queue payment")
	require.Equal(t, "pending", status)

	runnerCtx, stop := context.WithCancel(ctx)
	defer stop()

	runner := jobs.NewRunner("payment", env.app.JobStore, env.app.Jobs, jobs.Config{
		Interval:      50 * time.Millisecond,
		BatchSize:     10,
		LeaseDuration: 30 * time.Second,
		Concurrency:   2,
		PruneAge:      time.Hour,
		PruneLimit:    10,
	}, slog.New(slog.DiscardHandler))

	done := make(chan struct{})
	go func() {
		runner.Start(runnerCtx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		var s string
		if err := testPool.QueryRow(ctx,
			`SELECT status FROM job_queue WHERE dedup_key = $1`,
			"runner-claims:"+paymentID.String(),
		).Scan(&s); err != nil {
			return false
		}
		return s == "completed" || s == "dead" || s == "cancelled"
	}, 15*time.Second, 100*time.Millisecond, "the runner never settled the job")

	stop()
	<-done
}
