// Package bootstrap_test exercises bootstrap.New from outside, the way every
// real caller (server.go, cmd/worker) does: through its exported Deps and App.
package bootstrap_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_bootstrap")
	defer cleanup()
	testPool = pool

	os.Exit(m.Run())
}

// TestNewWiresOrderAndPaymentToEachOther pins the one wiring step New cannot
// express through constructor arguments alone: order and payment need each
// other, so SetOrderPaymentDeps closes that cycle after both exist. Skipping
// it leaves order.Service.payment nil, which panics the instant a caller
// reaches it -- so this test reaches it, on a real order, instead of checking
// the field is non-nil from outside.
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
	o := &order.Order{
		UserID:         userID,
		IdempotencyKey: userID.String(),
		RequestHash:    "hash-" + userID.String(),
		Status:         order.StatusAwaitingPayment,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
	}
	require.NoError(t, orderpg.New(testPool).Create(ctx, o))

	_, err = app.Orders.RetryPayment(ctx, userID, o.ID, "card")

	// A nil PaymentInitiator would panic inside RetryPayment and crash this
	// test outright; reaching a returned error at all proves SetOrderPaymentDeps
	// ran. The error itself is just the closed-port dial failing.
	require.Error(t, err)
}
