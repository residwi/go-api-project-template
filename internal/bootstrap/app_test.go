// Package bootstrap_test exercises bootstrap.New from outside, the way every
// real caller (server.go, cmd/worker) does: through its exported Deps and App.
package bootstrap_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_bootstrap")
	defer cleanup()
	testPool = pool

	os.Exit(m.Run())
}

// TestNewWiresOrderAndPaymentToEachOther pins the order/payment wiring across
// the module boundary. RetryPayment and Deps.Payment are gone -- checkout now
// owns the retry flow, rebuilding what it needs from order's query snapshot
// and payment's own Charge port -- so PaymentJobs is the only payment-shaped
// port left on order.Deps, feeding Cancel alone. At slice granularity the
// remaining cycle still runs through order/transition, order/query,
// order/cancel and payment/jobs: New builds order's shared reads and canceller
// first, then payment, then hands payment.Jobs to order's own constructor.
//
// Cancel's own PaymentJobs leg is nil-guarded (a dropped Deps field fails
// silently, not with a panic), so proof has to come from the database a real
// job row landed in, not from Execute's return value.
//
// The order and its payment job are seeded with raw SQL, not order's or
// payment's own adapters: domain.Order is module-private, so nothing outside
// order can construct one, and this package -- outside every module -- is no
// exception.
//
// This package owns Postgres database "test_bootstrap", so it is on the
// paralleltest exclusion list in .golangci.yml and does not call t.Parallel().
func TestNewWiresOrderAndPaymentToEachOther(t *testing.T) {
	testhelper.ResetDB(t, testPool)
	ctx := context.Background()

	app, err := bootstrap.New(bootstrap.Deps{
		Payment: payment.Config{
			// Port 1 is reserved and never listens, so the dial fails immediately
			// with a real error -- proof the port is wired, not a slow timeout.
			GatewayURL:     "http://127.0.0.1:1",
			GatewayTimeout: time.Second,
		},
		Pool:   testPool,
		Logger: testhelper.DiscardLogger(),
	})
	require.NoError(t, err)

	userID := testhelper.SeedUser(t, testPool)
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
		`INSERT INTO payment_jobs (payment_id, order_id, action, status) VALUES ($1, $2, 'charge', 'pending')`,
		paymentID, cancelOrderID,
	)
	require.NoError(t, err)

	require.NoError(t, app.Orders.Cancel.Execute(ctx, userID, cancelOrderID))

	var jobStatus string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT status FROM payment_jobs WHERE payment_id = $1`, paymentID,
	).Scan(&jobStatus))
	assert.Equal(t, "cancelled", jobStatus,
		"Cancel's PaymentJobs leg must be wired to the real payment.Module.Jobs by New")

	// checkout.Service.Snapshots is new in this refactor: order/usecase/query's
	// GetSnapshot now carries ID and UserID (it didn't when this test was first
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
