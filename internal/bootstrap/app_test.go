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
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/retrypayment"
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
// the module boundary: at slice granularity the cycle runs through four
// packages (order/transition, order/query, payment/charge, payment/jobs), not
// two, so New builds order's and payment's shared reads first, then payment,
// then hands payment.Charge and payment.Jobs to order's own constructor.
// Getting that sequencing wrong leaves order.Module's place/retrypayment/cancel
// slices with a nil PaymentInitiator, which panics the instant a caller
// reaches it -- so this test reaches it, on a real order, instead of checking
// the field is non-nil from outside.
//
// The order is seeded with raw SQL, not order's own adapters: domain.Order is
// module-private, so nothing outside order can construct one, and this
// package -- outside every module -- is no exception.
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
	var orderID uuid.UUID
	require.NoError(t, testPool.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, $3, 'awaiting_payment', 1000, 0, 1000, 'USD') RETURNING id`,
		userID,
		userID.String(),
		"hash-"+userID.String(),
	).Scan(&orderID))

	_, err = app.Orders.RetryPayment.Execute(ctx, userID, orderID, retrypayment.Params{PaymentMethodID: "card"})

	// A nil PaymentInitiator would panic inside Execute and crash this test
	// outright; reaching a returned error at all proves New wired payment.Charge
	// into order's constructor. The error itself is just the closed-port dial
	// failing.
	require.Error(t, err)
}
