package order

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

// TestNew_WiresPaymentJobsToCancel proves order.New forwards Deps.PaymentJobs
// into Cancel's own construction. Deps.Payment and the RetryPayment slice it
// fed are gone -- checkout reconstructs retry-payment from order's query
// snapshot and payment's own Charge port instead -- so PaymentJobCanceller is
// the only payment-shaped port left on order.Deps, and Cancel is its only
// consumer. Cancel's paymentCancel leg is nil-guarded, so a dropped Deps field
// would fail silently instead of panicking -- this drives Cancel's own
// Execute far enough to reach the call that Deps is supposed to have wired,
// which is what makes a dropped field fail this test via an unmet mock
// expectation rather than passing by accident.
//
// order.New always builds real postgres adapters from Deps.Pool -- module.go
// composes its own storage, so there is no seam to hand it a fake repository
// -- which is why this test needs a real database instead of a hand-rolled
// fake.
func TestNew_WiresPaymentJobsToCancel(t *testing.T) {
	t.Parallel()

	userID := seedUser(t)
	cancelOrderID := seedOrder(t, userID)

	paymentJobs := NewMockPaymentJobCanceller(t)

	m := New(Deps{
		Pool:             testPool,
		Tx:               database.NewTxRunner(testPool),
		Logger:           testhelper.DiscardLogger(),
		InventoryReserve: fanOutInventory{},
		InventoryDeduct:  fanOutInventory{},
		InventoryRestore: fanOutInventory{},
		Promotions:       fanOutCoupons{},
		PaymentJobs:      paymentJobs,
	})

	ctx := context.Background()

	paymentJobs.EXPECT().CancelPendingByOrderID(mock.Anything, cancelOrderID).Return(nil)
	require.NoError(
		t,
		m.Cancel.Execute(ctx, userID, cancelOrderID),
		"Cancel's payment leg must be wired by order.New",
	)
}

type fanOutInventory struct{}

func (fanOutInventory) ReserveBatch(context.Context, map[uuid.UUID]int) error { return nil }
func (fanOutInventory) DeductBatch(context.Context, map[uuid.UUID]int) error  { return nil }
func (fanOutInventory) Restore(context.Context, map[uuid.UUID]int, inventorycontract.StockState) error {
	return nil
}

type fanOutCoupons struct{}

func (fanOutCoupons) Reserve(context.Context, string, uuid.UUID, uuid.UUID, int64) (int64, error) {
	return 0, nil
}
func (fanOutCoupons) Release(context.Context, uuid.UUID) error { return nil }

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

// seedOrder seeds an order in awaiting_payment, the state Cancel's own
// transition guard requires.
func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
	return id
}
