package order

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/place"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/retrypayment"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
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

// TestNew_WiresPaymentDeps proves the fan-out that a since-deleted setter
// method used to perform now happens at construction: Place, RetryPayment and
// Cancel each panic or mock-fail if order.New did not forward Deps.Payment
// and Deps.PaymentJobs to them. Cancel's paymentCancel is nil-guarded, so a
// dropped leg there fails silently instead of panicking -- this drives each
// slice's own Execute far enough to reach the payment call that Deps is
// supposed to have wired, which is what makes a dropped leg fail this test:
// by panic for Place/RetryPayment, by an unmet mock expectation for Cancel.
//
// order.New always builds real postgres adapters from Deps.Pool -- module.go
// composes its own storage, so there is no seam to hand it a fake repository
// -- which is why this test needs a real database instead of the hand-rolled
// fakes the old setter-based version used.
func TestNew_WiresPaymentDeps(t *testing.T) {
	t.Parallel()

	userID := seedUser(t)
	productID := seedProduct(t)
	retryOrderID := seedOrder(t, userID)
	cancelOrderID := seedOrder(t, userID)

	payment := NewMockPaymentInitiator(t)
	paymentJobs := NewMockPaymentJobCanceller(t)

	m := New(Deps{
		Pool:             testPool,
		Tx:               database.NewTxRunner(testPool),
		Logger:           testhelper.DiscardLogger(),
		Cart:             &fanOutCartProvider{productID: productID},
		InventoryReserve: fanOutInventory{},
		InventoryDeduct:  fanOutInventory{},
		InventoryRestore: fanOutInventory{},
		Promotions:       fanOutCoupons{},
		Notifications:    fanOutNotifications{},
		Payment:          payment,
		PaymentJobs:      paymentJobs,
	})

	ctx := context.Background()

	payment.EXPECT().
		InitiatePayment(mock.Anything, mock.MatchedBy(func(r paymentcontract.ChargeRequest) bool {
			return r.PaymentMethodID == "pm_fanout_place"
		})).
		Return(paymentcontract.ChargeResult{}, nil)
	_, err := m.Place.Execute(ctx, userID, place.Params{PaymentMethodID: "pm_fanout_place"}, "idem-fanout-place")
	require.NoError(t, err, "Place's payment leg must be wired by order.New")

	payment.EXPECT().
		InitiatePayment(mock.Anything, mock.MatchedBy(func(r paymentcontract.ChargeRequest) bool {
			return r.OrderID == retryOrderID
		})).
		Return(paymentcontract.ChargeResult{}, nil)
	_, err = m.RetryPayment.Execute(ctx, userID, retryOrderID, retrypayment.Params{PaymentMethodID: "pm_fanout_retry"})
	require.NoError(t, err, "RetryPayment's payment leg must be wired by order.New")

	paymentJobs.EXPECT().CancelPendingByOrderID(mock.Anything, cancelOrderID).Return(nil)
	require.NoError(
		t,
		m.Cancel.Execute(ctx, userID, cancelOrderID),
		"Cancel's payment leg must be wired by order.New",
	)
}

type fanOutCartProvider struct{ productID uuid.UUID }

func (f *fanOutCartProvider) LockCart(context.Context, uuid.UUID) error { return nil }
func (f *fanOutCartProvider) GetSnapshot(context.Context, uuid.UUID) (*cartcontract.Cart, error) {
	return &cartcontract.Cart{
		ID: uuid.New(),
		Items: []cartcontract.CartItem{
			{ProductID: f.productID, Quantity: 1, Name: "Widget", Price: money.New(1000, "USD"), Status: "published"},
		},
	}, nil
}
func (f *fanOutCartProvider) Clear(context.Context, uuid.UUID) error { return nil }

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

type fanOutNotifications struct{}

func (fanOutNotifications) EnqueueOrderPlaced(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

// seedOrder seeds an order in awaiting_payment: fresh for RetryPayment and
// Cancel to each own, since neither may collide with the other's write.
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
