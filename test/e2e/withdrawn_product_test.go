package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apihttp "github.com/residwi/go-api-project-template/internal/transport/http"
)

func TestE2ECheckoutRejectsWithdrawnProduct(t *testing.T) {
	setup(t)
	handler := apihttp.NewRouter(testDeps, testApp)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Withdrawn Cat', $2, true)`,
		catID, "withdrawn-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Withdrawn Widget', $2, 'desc', 5000, 'USD', 'published', $3)`,
		prodID, "withdrawn-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	const email = "e2e-withdrawn@example.com"
	regBody := `{"email":"` + email + `","password":"Password123!","first_name":"E2E","last_name":"User"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.ServeHTTP(regW, regReq)
	require.Equal(t, http.StatusCreated, regW.Code)

	var regResp map[string]any
	require.NoError(t, json.NewDecoder(regW.Body).Decode(&regResp))
	token := regResp["data"].(map[string]any)["access_token"].(string)
	t.Cleanup(func() {
		testPool.Exec(
			ctx,
			`DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM carts WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(
			ctx,
			`DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1))`,
			email,
		)
		testPool.Exec(ctx, `DELETE FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})

	// The product is live at this point, so adding it must succeed.
	addBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("Authorization", "Bearer "+token)
	addW := httptest.NewRecorder()
	handler.ServeHTTP(addW, addReq)
	require.Equal(t, http.StatusCreated, addW.Code, "the product is still live, so adding it must succeed")

	// Withdraw it exactly as product.Delete does: deleted_at only, status untouched.
	tag, err := testPool.Exec(ctx, `UPDATE products SET deleted_at = NOW() WHERE id = $1`, prodID)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected(), "the withdrawal must actually land")

	orderBody := `{"payment_method_id":"pm_test_123"}`
	orderReq := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(orderBody))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	orderReq.Header.Set("Idempotency-Key", uuid.New().String())
	orderW := httptest.NewRecorder()
	handler.ServeHTTP(orderW, orderReq)

	require.Equal(t, http.StatusBadRequest, orderW.Code,
		"a soft-deleted product's row still reads status='published' -- checkout must reject it, not sell it")

	var count int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, email,
	).Scan(&count))
	assert.Zero(t, count, "a rejected checkout must leave no order row behind")
}
