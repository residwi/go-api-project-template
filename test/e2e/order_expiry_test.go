package e2e_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/bootstrap"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	inventorypg "github.com/residwi/go-api-project-template/internal/modules/inventory/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	orderpg "github.com/residwi/go-api-project-template/internal/modules/order/postgres"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// newExpiryService wires order to a real inventory service over the test DB.
// ExpireStale touches only the repo and inventory deps, so cart, promotion and
// notification are left nil.
func newExpiryService(t *testing.T) *order.Service {
	t.Helper()
	return bootstrap.NewOrderService(
		orderpg.New(testPool),
		database.NewTxRunner(testPool),
		nil,
		inventory.NewService(inventorypg.New(testPool)),
		nil,
		nil,
	)
}

func seedExpiryUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedAwaitingPaymentOrder(t *testing.T, userID uuid.UUID) *order.Order {
	t.Helper()
	ctx := context.Background()
	key := uuid.New().String()
	o := &order.Order{
		UserID:         userID,
		IdempotencyKey: key,
		RequestHash:    "hash-" + key,
		Status:         order.StatusAwaitingPayment,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
	}
	require.NoError(t, orderpg.New(testPool).Create(ctx, o))
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, o.ID) })
	return o
}

func orderStatusOf(t *testing.T, orderID uuid.UUID) order.Status {
	t.Helper()
	var status order.Status
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
	return status
}

func TestE2EOrderExpiry(t *testing.T) {
	t.Run("expires a stale order and releases its reservation", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		userID := seedExpiryUser(t)

		productID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO products (id, name, slug, price, currency, status)
			 VALUES ($1, 'Expiry Product', $2, 1000, 'USD', 'published')`,
			productID, "expiry-"+productID.String()[:8])
		require.NoError(t, err)
		// inventory reads and writes inventory_levels, not products:
		// 7 sellable plus a 3-unit hold for the order placed below.
		seedInventoryLevel(t, productID, 7, 3)
		t.Cleanup(func() {
			testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, productID)
			testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, productID)
		})

		o := seedAwaitingPaymentOrder(t, userID)
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

		_, reserved := inventoryLevelOf(t, productID)
		assert.Equal(t, 0, reserved)
	})

	t.Run("leaves a recent order untouched", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		userID := seedExpiryUser(t)
		o := seedAwaitingPaymentOrder(t, userID)

		require.NoError(t, newExpiryService(t).ExpireStale(ctx))

		assert.Equal(t, order.StatusAwaitingPayment, orderStatusOf(t, o.ID))
	})
}
