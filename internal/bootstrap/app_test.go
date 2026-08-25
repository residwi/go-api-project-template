// Package bootstrap_test exercises bootstrap.New from outside, the way every
// real caller (server.go, cmd/worker) does: through its exported constructor
// and App.
package bootstrap_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_bootstrap")
	defer cleanup()
	testPool = pool

	os.Exit(m.Run())
}

// TestNewWiresOrderAndPaymentToEachOther pins the order/payment wiring across
// the module boundary. RetryPayment and Deps.Payment are gone -- checkout now
// owns the retry flow, rebuilding what it needs from order's Snapshot
// projection and payment's own Charge port -- and the payment-job-cancel edge
// that used to live on order.Deps.PaymentJobs has moved the same way:
// order.Service.CancelByUser now only reverses stock and releases the coupon,
// checkout.Payments.CancelPendingByOrderID feeds checkout.Service.CancelOrder
// instead.
// Order still feeds payment three ports, but order has no payment-shaped
// dependency left, so New builds order.Service first and hands payment.New
// that one value for all three -- the same instance order's own writes use,
// not the pair of standalone builds this test used to have to trust were
// equivalent. Checkout is built last, on top of both.
//
// checkout.Service.CancelOrder's payment-job leg is best-effort and swallows
// its own error, so proof has to come from the database a real job row
// landed in, not from CancelOrder's return value.
//
// The order and its payment job are seeded with raw SQL, not order's or
// payment's own adapters: domain.Order is module-private, so nothing outside
// order can construct one, and this package -- outside every module -- is no
// exception.
//
// This package owns Postgres database "test_bootstrap", so it is on the
// paralleltest exclusion list in .golangci.yml and does not call t.Parallel().
func TestNewWiresOrderAndPaymentToEachOther(t *testing.T) {
	testutil.ResetDB(t, testPool)
	ctx := context.Background()

	var cache *redis.Client
	app, err := bootstrap.New(
		auth.Config{},
		cart.Config{},
		payment.Config{
			// Port 1 is reserved and never listens, so the dial fails immediately
			// with a real error -- proof the port is wired, not a slow timeout.
			GatewayURL:     "http://127.0.0.1:1",
			GatewayTimeout: time.Second,
		},
		database.DB{Primary: testPool},
		cache,
		testutil.DiscardLogger(),
	)
	require.NoError(t, err)

	userID := testutil.SeedUser(t, testPool)
	seedOrder := func() uuid.UUID {
		var orderID uuid.UUID
		require.NoError(t, testPool.QueryRow(
			ctx,
			`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, $3, 'awaiting_payment', 1000, 0, 1000, 'USD') RETURNING id`,
			userID,
			uuid.New().String(),
			"hash-"+uuid.New().String(),
		).Scan(&orderID))
		return orderID
	}

	cancelOrderID := seedOrder()
	var paymentID uuid.UUID
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO payments (order_id, amount, currency, status) VALUES ($1, 1000, 'USD', 'pending') RETURNING id`,
		cancelOrderID,
	).Scan(&paymentID))
	_, err = testPool.Exec(ctx,
		`INSERT INTO job_queue (queue, kind, payload, group_key) VALUES ('payment', 'payment.refund', '{}', $1)`,
		"order:"+cancelOrderID.String(),
	)
	require.NoError(t, err)

	require.NoError(t, app.Checkout.CancelOrder(ctx, userID, cancelOrderID))

	var jobStatus string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT status FROM job_queue WHERE group_key = $1`, "order:"+cancelOrderID.String(),
	).Scan(&jobStatus))
	assert.Equal(t, "cancelled", jobStatus,
		"checkout.Service.CancelOrder must be wired to both order.Service and payment.Service by New")

	// checkout.Service.Snapshots is new in this refactor: order.Service.Snapshot
	// carries ID and UserID (it didn't when this test was first
	// written -- see task-3-report.md for that incident), so a real caller's
	// ownership check correctly passes here and RetryPayment reaches
	// payment.Charge. A separate, untouched order is used rather than
	// cancelOrderID above, so this assertion is not tangled up with Cancel's
	// own status transition.
	retryOrderID := seedOrder()
	_, err = app.Checkout.RetryPayment(ctx, userID, retryOrderID, "card")

	// A nil Snapshots or Payments port would panic; an ownership check that
	// still lost against the real caller would return apperror.ErrNotFound
	// instead. Reaching the closed-port dial failure proves both legs wired.
	require.Error(t, err)
	require.NotErrorIs(t, err, apperror.ErrNotFound)
}
