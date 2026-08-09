package order

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	cartcontract "github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/cancel"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/place"
	"github.com/residwi/go-api-project-template/internal/modules/order/retrypayment"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// TestModule_SetPaymentDeps proves the fan-out itself, not each slice's own
// setter (already covered in place/cancel/retrypayment's own tests): Place and
// RetryPayment panic on a nil PaymentInitiator, and Cancel's paymentCancel is
// nil-guarded, so a dropped leg there fails silently instead. Each of the
// three assertions below drives its slice's own Execute far enough to reach
// the payment call SetPaymentDeps is supposed to have wired, so dropping any
// one of the three fan-out calls in Module.SetPaymentDeps fails this test --
// by panic for Place/RetryPayment, by an unmet mock expectation for Cancel.
func TestModule_SetPaymentDeps(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	productID := uuid.New()
	placeOrderID := uuid.New()
	retryOrderID := uuid.New()
	cancelOrderID := uuid.New()

	m := &Module{
		Place: place.New(
			&fanOutPlaceRepo{orderID: placeOrderID},
			testhelper.FakeTxRunner{},
			&fanOutCartProvider{productID: productID},
			fanOutInventory{},
			fanOutCoupons{},
			fanOutNotifications{},
			fanOutTransition{},
			testhelper.DiscardLogger(),
		),
		RetryPayment: retrypayment.New(&fanOutRetryRepo{orderID: retryOrderID, userID: userID}),
		Cancel: cancel.New(
			&fanOutCancelRepo{orderID: cancelOrderID, userID: userID},
			testhelper.FakeTxRunner{},
			fanOutTransition{},
			fanOutInventory{},
			nil,
			testhelper.DiscardLogger(),
		),
	}

	payment := NewMockPaymentInitiator(t)
	paymentCancel := NewMockPaymentJobCanceller(t)
	m.SetPaymentDeps(payment, paymentCancel)

	ctx := context.Background()

	payment.EXPECT().
		InitiatePayment(mock.Anything, mock.MatchedBy(func(r paymentcontract.ChargeRequest) bool {
			return r.OrderID == placeOrderID
		})).
		Return(paymentcontract.ChargeResult{}, nil)
	_, err := m.Place.Execute(ctx, userID, place.Params{PaymentMethodID: "pm_test"}, "idem-fanout-place")
	require.NoError(t, err, "Place's payment leg must be wired by SetPaymentDeps")

	payment.EXPECT().
		InitiatePayment(mock.Anything, mock.MatchedBy(func(r paymentcontract.ChargeRequest) bool {
			return r.OrderID == retryOrderID
		})).
		Return(paymentcontract.ChargeResult{}, nil)
	_, err = m.RetryPayment.Execute(ctx, userID, retryOrderID, retrypayment.Params{PaymentMethodID: "pm_test"})
	require.NoError(t, err, "RetryPayment's payment leg must be wired by SetPaymentDeps")

	paymentCancel.EXPECT().CancelJobsByOrderID(mock.Anything, cancelOrderID).Return(nil)
	require.NoError(
		t,
		m.Cancel.Execute(ctx, userID, cancelOrderID),
		"Cancel's payment leg must be wired by SetPaymentDeps",
	)
}

type fanOutPlaceRepo struct{ orderID uuid.UUID }

func (f *fanOutPlaceRepo) Create(_ context.Context, o *domain.Order) error {
	o.ID = f.orderID
	return nil
}
func (f *fanOutPlaceRepo) CreateItems(context.Context, []domain.Item) error { return nil }
func (f *fanOutPlaceRepo) GetByUserIDAndIdempotencyKey(context.Context, uuid.UUID, string) (*domain.Order, error) {
	return nil, apperror.ErrNotFound
}

func (f *fanOutPlaceRepo) ListItemsByOrderID(context.Context, uuid.UUID) ([]domain.Item, error) {
	return nil, nil
}

func (f *fanOutPlaceRepo) UpdateTotals(context.Context, uuid.UUID, int64, int64) error { return nil }

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

// fanOutInventory satisfies both place.InventoryReserver and
// cancel.InventoryRestorer -- the two only overlap on shape, never on a
// shared instance in production.
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

type fanOutNotifications struct{}

func (fanOutNotifications) EnqueueOrderPlaced(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

// fanOutTransition satisfies place.TransitionApplier and
// cancel.TransitionApplier, both a bare Apply.
type fanOutTransition struct{}

func (fanOutTransition) Apply(context.Context, uuid.UUID, domain.Transition) error { return nil }

type fanOutRetryRepo struct{ orderID, userID uuid.UUID }

func (f *fanOutRetryRepo) GetByID(context.Context, uuid.UUID) (*domain.Order, error) {
	return &domain.Order{
		ID: f.orderID, UserID: f.userID, Status: domain.StatusAwaitingPayment, Total: money.New(1000, "USD"),
	}, nil
}

type fanOutCancelRepo struct{ orderID, userID uuid.UUID }

func (f *fanOutCancelRepo) GetByID(context.Context, uuid.UUID) (*domain.Order, error) {
	return &domain.Order{ID: f.orderID, UserID: f.userID, Status: domain.StatusAwaitingPayment}, nil
}

// ListItemsByOrderID returns none, on purpose: this test is about the payment
// leg, so it keeps inventory.Restore out of the picture entirely rather than
// mocking a call this test does not care about.
func (f *fanOutCancelRepo) ListItemsByOrderID(context.Context, uuid.UUID) ([]domain.Item, error) {
	return nil, nil
}
