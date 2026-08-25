package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2EFullyDiscountedOrder drives the zero-total path: a 100% coupon takes
// the order total to nothing, so order.Service skips the payment tail entirely
// and finalizes the order itself -- marking it paid and turning the reservation
// into a deduction. That branch is the only reader of order.Deps.InventoryDeduct,
// and deleting that assignment in internal/bootstrap/app.go used to leave every
// e2e green.
func TestE2EFullyDiscountedOrder(t *testing.T) {
	setup(t)

	// Default gateway config points at a dead port: nothing here may charge.
	handler := newTestRouter(testPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Free Cat', $2, true)`,
		catID, "free-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Free Product', $2, 'desc', 1500, 'USD', 'published', $3)`,
		prodID, "free-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 10, 0)

	couponID := uuid.New()
	couponCode := "FREE" + couponID.String()[:8]
	_, err = testPool.Exec(
		ctx,
		`INSERT INTO promotions (id, code, type, value, min_order_amount, max_uses, used_count, starts_at, expires_at, active)
		 VALUES ($1, $2, 'percentage', 100, 0, 10, 0, $3, $4, true)`,
		couponID,
		couponCode,
		time.Now().Add(-time.Hour),
		time.Now().Add(time.Hour),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE coupon_id = $1`, couponID)
		testPool.Exec(ctx, `DELETE FROM promotions WHERE id = $1`, couponID)
	})

	userID, token := registerE2EUser(t, handler, "free-order@example.com")
	t.Cleanup(func() { cleanupOrdersOf(userID) })

	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items",
		strings.NewReader(`{"product_id":"`+prodID.String()+`","quantity":2}`))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	orderReq := httptest.NewRequest(http.MethodPost, "/api/orders",
		strings.NewReader(fmt.Sprintf(`{"payment_method_id":"pm_test_free","coupon_code":"%s"}`, couponCode)))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	orderReq.Header.Set("Idempotency-Key", uuid.New().String())
	orderW := httptest.NewRecorder()
	handler.ServeHTTP(orderW, orderReq)
	require.Equal(t, http.StatusCreated, orderW.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(orderW.Body.Bytes(), &resp))
	orderData := resp["data"].(map[string]any)["order"].(map[string]any)
	orderID := uuid.MustParse(orderData["id"].(string))

	t.Run("the discount covers the whole order", func(t *testing.T) {
		assert.InDelta(t, float64(3000), orderData["subtotal_amount"], 0.0001)
		assert.InDelta(t, float64(3000), orderData["discount_amount"], 0.0001)
		assert.InDelta(t, float64(0), orderData["total_amount"], 0.0001)
	})

	t.Run("a zero-total order is paid without a payment", func(t *testing.T) {
		assert.Equal(t, "paid", orderStatusOf(t, orderID))
		assert.Equal(t, 0, countRows(t, `SELECT COUNT(*) FROM payments WHERE order_id = $1`, orderID))
	})

	t.Run("finalizing it deducts the stock it had reserved", func(t *testing.T) {
		available, reserved := inventoryLevelOf(t, prodID)
		assert.Equal(t, 8, available)
		assert.Equal(t, 0, reserved)
	})
}
