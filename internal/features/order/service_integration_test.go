package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/order"
)

// expireInventory adapts the real inventory.Service to order.InventoryReserver,
// translating the cross-feature wasDeducted bool to inventory's StockState — the
// same job the production wiring adapter does, kept local so order's tests don't
// depend on the wiring layer.
type expireInventory struct{ svc *inventory.Service }

func (a expireInventory) ReserveBatch(ctx context.Context, items []order.InventoryItem) error {
	return a.svc.ReserveBatch(ctx, toStockChanges(items))
}

func (a expireInventory) DeductBatch(ctx context.Context, items []order.InventoryItem) error {
	return a.svc.DeductBatch(ctx, toStockChanges(items))
}

func (a expireInventory) Restore(ctx context.Context, items []order.InventoryItem, wasDeducted bool) error {
	state := inventory.Reserved
	if wasDeducted {
		state = inventory.Deducted
	}
	return a.svc.Restore(ctx, toStockChanges(items), state)
}

func toStockChanges(items []order.InventoryItem) []inventory.StockChange {
	changes := make([]inventory.StockChange, len(items))
	for i, it := range items {
		changes[i] = inventory.StockChange{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	return changes
}

// newExpiryService wires order to a real inventory service over the test DB.
// ExpireStale uses only the repo and inventory deps, so cart/payment/coupon/
// notification are left nil.
func newExpiryService(t *testing.T) *order.Service {
	t.Helper()
	orderRepo := order.NewPostgresRepository(testPool)
	invSvc := inventory.NewService(inventory.NewPostgresRepository(testPool), testPool)
	return order.NewService(orderRepo, testPool, nil, expireInventory{svc: invSvc}, nil, nil, nil, nil)
}

func orderStatusOf(t *testing.T, orderID uuid.UUID) order.Status {
	t.Helper()
	var status order.Status
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
	return status
}

func TestService_ExpireStale_Integration(t *testing.T) {
	t.Run("expires a stale order and releases its reservation", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		userID := seedUser(t)

		productID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, price, currency, status, stock_quantity, reserved_quantity)
			 VALUES ($1, 'Expiry Product', $2, 1000, 'USD', 'published', 10, 3)`,
			productID, "expiry-"+productID.String()[:8])
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, productID) })

		o := seedOrder(t, userID)
		_, err = testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Expiry Product', 1000, 3, 3000)`, o.ID, productID)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, o.ID) })

		// Age the order past the 30-minute payment window.
		_, err = testPool.Exec(ctx,
			`UPDATE orders SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, o.ID)
		require.NoError(t, err)

		require.NoError(t, newExpiryService(t).ExpireStale(ctx))

		assert.Equal(t, order.StatusExpired, orderStatusOf(t, o.ID))

		var reserved int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT reserved_quantity FROM products WHERE id = $1`, productID).Scan(&reserved))
		assert.Equal(t, 0, reserved)
	})

	t.Run("leaves a recent order untouched", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		userID := seedUser(t)
		o := seedOrder(t, userID)

		require.NoError(t, newExpiryService(t).ExpireStale(ctx))

		assert.Equal(t, order.StatusAwaitingPayment, orderStatusOf(t, o.ID))
	})
}
