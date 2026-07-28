package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/product"
)

// TestToProductResponse_OmitsReservationAndSoftDeleteState pins the Phase 2
// decision that reservation counts are live order velocity per SKU and must
// never reach a product response, public or admin. StockQuantity is sourced
// from Availability.OnHand -- a distinguishable Available value proves the
// mapper is not accidentally reading the wrong side of Availability -- and
// DeletedAt must never distinguish a soft-deleted product from one that
// simply 404s.
func TestToProductResponse_OmitsReservationAndSoftDeleteState(t *testing.T) {
	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toProductResponse(&product.Product{
		ID:        uuid.New(),
		Name:      "Widget",
		Slug:      "widget",
		Price:     1999,
		Currency:  "USD",
		Status:    "published",
		DeletedAt: &deletedAt,
		Availability: product.Availability{
			OnHand:    50,
			Available: 424242, // distinguishable -- proves StockQuantity reads OnHand, not Available
		},
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "name", "slug", "price", "currency", "status", "stock_quantity", "created_at", "updated_at"},
		keysOf(fields),
		"description, category_id, compare_at_price, sku, and images are omitempty and absent when nil/empty")

	assert.NotContains(t, string(raw), "424242",
		"reserved/available stock is live order velocity per SKU and must never be serialised")
	assert.NotContains(t, string(raw), "2026-01-01",
		"a soft-deleted product must not be distinguishable on the wire from one that 404s")

	var stock struct {
		StockQuantity int `json:"stock_quantity"`
	}
	require.NoError(t, json.Unmarshal(raw, &stock))
	assert.Equal(t, 50, stock.StockQuantity, "stock_quantity must come from Availability.OnHand")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
