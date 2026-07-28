package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/order"
)

// TestAddressResponse_JSONRoundTrip pins the wire shape order.Address used to
// own directly (address_test.go, pre-refactor) -- now addressResponse's job.
func TestAddressResponse_JSONRoundTrip(t *testing.T) {
	got := toAddressResponse(&order.Address{
		Street:  "123 Main St",
		City:    "Springfield",
		State:   "IL",
		ZipCode: "62701",
		Country: "US",
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"street":"123 Main St",
		"city":"Springfield",
		"state":"IL",
		"zip_code":"62701",
		"country":"US"
	}`, string(raw))
}

func TestAddressResponse_NilIsNil(t *testing.T) {
	assert.Nil(t, toAddressResponse(nil))
}

// TestToOrderResponse_OmitsSagaAndIdempotencyInternals pins the plan's
// callout: RequestHash is an idempotency internal, and StockDeducted/
// StockReversed are saga state that would let a client infer fulfilment
// internals if published. Item.OrderID is dropped too -- it's an internal
// join key, and an item is always returned nested inside its own order.
func TestToOrderResponse_OmitsSagaAndIdempotencyInternals(t *testing.T) {
	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toOrderResponse(&order.Order{
		ID:             orderID,
		UserID:         uuid.New(),
		IdempotencyKey: "idem-key-1",
		RequestHash:    "distinguishable-request-hash", // internal -- must not reach the wire
		Status:         order.StatusPaid,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
		StockDeducted:  true, // internal -- must not reach the wire
		StockReversed:  true, // internal -- must not reach the wire
		Items: []order.Item{
			{ID: uuid.New(), OrderID: orderID, ProductID: uuid.New(), ProductName: "Widget", Price: money.New(1000, "USD"), Quantity: 1, Subtotal: money.New(1000, "USD"), CreatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "user_id", "status", "subtotal_amount", "discount_amount", "total_amount",
			"currency", "items", "created_at", "updated_at",
		},
		keysOf(fields),
		"idempotency_key, request_hash, stock_deducted, and stock_reversed must not appear")

	// Each money.Money is flattened to its minor-unit amount, with the currency
	// emitted once at order level. A nested {"Amount":..,"Currency":..} object
	// here -- i.e. a Money that marshalled itself -- would fail both assertions.
	assert.JSONEq(t, `1000`, string(fields["total_amount"]))
	// Pins that a currency is emitted, not that it comes from Total specifically:
	// the fixture denominates all three amounts "USD", so Subtotal's or
	// Discount's would satisfy this identically. That is fine -- the mapper's own
	// comment says any of the three would do, because PlaceOrder builds them from
	// one currency and the row stores one column. Stated so nobody reads this as
	// a guarantee about which field the mapper reads.
	assert.JSONEq(t, `"USD"`, string(fields["currency"]))

	assert.NotContains(t, string(raw), "distinguishable-request-hash",
		"RequestHash is an idempotency internal and must not be serialised")
	assert.NotContains(t, string(raw), "idem-key-1",
		"IdempotencyKey must not be serialised")
	assert.NotContains(t, string(raw), "stock_deducted",
		"StockDeducted is saga state and must not be serialised")
	assert.NotContains(t, string(raw), "stock_reversed",
		"StockReversed is saga state and must not be serialised")

	var itemFields []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["items"], &itemFields))
	require.Len(t, itemFields, 1)
	assert.ElementsMatch(t,
		[]string{"id", "product_id", "product_name", "price", "quantity", "subtotal", "created_at"},
		keysOf(itemFields[0]),
		"order_id must not appear on a line item -- it's an internal join key")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
