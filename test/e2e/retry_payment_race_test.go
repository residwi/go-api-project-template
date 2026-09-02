package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// RetryPayment used to read the order status and then charge. Two requests both
// passed that check and the card was billed twice, and no test caught it because
// every other e2e drives one request at a time.
func TestE2ERetryPaymentRace(t *testing.T) {
	setup(t)

	var charges atomic.Int64
	mockMux := http.NewServeMux()
	mockgatewayserver.RegisterRoutes(mockMux, testutil.DiscardLogger())
	// The delay holds the winner inside the gateway so both retries are genuinely
	// in flight at once. Without it the first completes and releases its claim
	// before the second asks for one, and two sequential retries charging twice
	// is correct behaviour rather than the bug.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/charge") {
			charges.Add(1)
			time.Sleep(300 * time.Millisecond)
		}
		mockMux.ServeHTTP(w, r)
	}))
	defer mockServer.Close()

	customPaymentCfg := payment.Config{
		Gateway:        "mock",
		GatewayURL:     mockServer.URL + "/mock/payment",
		GatewayTimeout: 10 * time.Second,
	}
	handler := newTestRouter(customPaymentCfg)
	ctx := context.Background()

	catID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO categories (id, name, slug, active) VALUES ($1, 'Race Cat', $2, true)`,
		catID, "race-cat-"+catID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, catID) })

	// 1099 ends in 99, so the mock gateway rejects every charge and the order is
	// left awaiting_payment -- retryable, which is the state this race needs.
	prodID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status, category_id)
		 VALUES ($1, 'Race Product', $2, 'desc', 1099, 'USD', 'published', $3)`,
		prodID, "race-prod-"+prodID.String()[:8], catID)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM inventory_levels WHERE product_id = $1`, prodID)
		testPool.Exec(ctx, `DELETE FROM products WHERE id = $1`, prodID)
	})
	seedInventoryLevel(t, prodID, 100, 0)

	userID, token := registerE2EUser(t, handler, "retry-race@example.com")
	t.Cleanup(func() { cleanupOrdersOf(userID) })

	cartBody := `{"product_id":"` + prodID.String() + `","quantity":1}`
	cartReq := httptest.NewRequest(http.MethodPost, "/api/cart/items", strings.NewReader(cartBody))
	cartReq.Header.Set("Content-Type", "application/json")
	cartReq.Header.Set("Authorization", "Bearer "+token)
	cartW := httptest.NewRecorder()
	handler.ServeHTTP(cartW, cartReq)
	require.Equal(t, http.StatusCreated, cartW.Code)

	placeReq := httptest.NewRequest(http.MethodPost, "/api/orders",
		strings.NewReader(`{"payment_method_id":"pm_test_race"}`))
	placeReq.Header.Set("Content-Type", "application/json")
	placeReq.Header.Set("Authorization", "Bearer "+token)
	placeReq.Header.Set("Idempotency-Key", uuid.New().String())
	placeW := httptest.NewRecorder()
	handler.ServeHTTP(placeW, placeReq)
	require.Equal(t, http.StatusCreated, placeW.Code)

	orderID := singleOrderIDOf(t, userID)
	require.Equal(t, "awaiting_payment", orderStatusOf(t, orderID))
	require.EqualValues(t, 1, charges.Load(), "placement charges once and the gateway rejects it")

	t.Run("only one of two concurrent retries reaches the gateway", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 2 {
			wg.Go(func() {
				req := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID.String()+"/pay",
					strings.NewReader(`{"payment_method_id":"pm_test_race"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				<-start
				handler.ServeHTTP(httptest.NewRecorder(), req)
			})
		}
		close(start)
		wg.Wait()

		assert.EqualValues(t, 2, charges.Load(), "the loser must not charge the card a second time")
		assert.Equal(t, "awaiting_payment", orderStatusOf(t, orderID),
			"the winner released its claim when the charge failed")
		assert.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM payments WHERE order_id = $1`, orderID))
	})
}

func singleOrderIDOf(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT id FROM orders WHERE user_id = $1`, userID).Scan(&id))
	return id
}
