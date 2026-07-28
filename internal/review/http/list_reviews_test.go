package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/review"
)

// TestToReviewResponse_OmitsReviewerAndInternalFields pins the plan's privacy
// callout: naming the reviewer's id on a public response would let a scraper
// correlate purchases to accounts, so UserID must never reach the wire.
// OrderID (purchase-verification-only) and Status/UpdatedAt (constant --
// every review this feature can return is already published) are dropped
// alongside it.
func TestToReviewResponse_OmitsReviewerAndInternalFields(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toReviewResponse(review.Review{
		ID:        uuid.New(),
		UserID:    userID, // internal -- must not reach the wire
		ProductID: uuid.New(),
		OrderID:   orderID, // internal -- must not reach the wire
		Rating:    5,
		Title:     "Great",
		Body:      "Love it",
		Status:    "published",
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "product_id", "rating", "title", "body", "created_at"}, keysOf(fields),
		"the response must expose exactly these fields")

	assert.NotContains(t, string(raw), userID.String(),
		"a review response naming the reviewer's id lets a scraper correlate purchases to accounts")
	assert.NotContains(t, string(raw), orderID.String(),
		"order_id exists only to verify provenance at creation time; a client has no use for it back")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
