package e2e_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// ExpireStale touches only the repo and inventory deps -- testApp.Orders is
// fully wired, but releaseOrderHolds guards its coupon release on a non-empty
// CouponCode, which the orders seeded below never set, so cart, promotion and
// notification never fire.
func newExpiryService(t *testing.T) *order.Module {
	t.Helper()
	return testApp.Orders
}

func seedExpiryUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

// seedAwaitingPaymentOrder is raw SQL, not order's own adapters: domain.Order
// is module-private, so nothing outside order can construct one.
func seedAwaitingPaymentOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	key := uuid.New().String()
	var orderID uuid.UUID
	require.NoError(t, testPool.QueryRow(
		ctx,
		`INSERT INTO orders (user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, $3, 'awaiting_payment', 1000, 0, 1000, 'USD') RETURNING id`,
		userID,
		key,
		"hash-"+key,
	).Scan(&orderID))
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}

func orderStatusOf(t *testing.T, orderID uuid.UUID) string {
	t.Helper()
	var status string
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

		orderID := seedAwaitingPaymentOrder(t, userID)
		_, err = testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Expiry Product', 1000, 3, 3000)`, orderID, productID)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID) })

		// Age the order past the 30-minute payment window.
		_, err = testPool.Exec(ctx,
			`UPDATE orders SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, orderID)
		require.NoError(t, err)

		require.NoError(t, newExpiryService(t).Expire.ExpireStale(ctx))

		assert.Equal(t, "expired", orderStatusOf(t, orderID))

		available, reserved := inventoryLevelOf(t, productID)
		assert.Equal(t, 10, available) // 7 seeded + 3 released back
		assert.Equal(t, 0, reserved)
	})

	t.Run("leaves a recent order untouched", func(t *testing.T) {
		setup(t)
		ctx := context.Background()
		userID := seedExpiryUser(t)
		orderID := seedAwaitingPaymentOrder(t, userID)

		require.NoError(t, newExpiryService(t).Expire.ExpireStale(ctx))

		assert.Equal(t, "awaiting_payment", orderStatusOf(t, orderID))
	})
}
