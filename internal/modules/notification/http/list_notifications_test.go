package http

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification"
)

// TestToNotificationResponse_OmitsUserIDAndRawPayload pins the plan's
// callout: Data []byte is a raw payload and must never reach the wire as
// raw bytes. UserID is dropped too -- the caller is always the
// authenticated user, so echoing it back adds nothing.
func TestToNotificationResponse_OmitsUserIDAndRawPayload(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data := []byte(`{"order_id":"distinguishable-raw-payload"}`) // must not reach the wire

	got := toNotificationResponse(notification.Notification{
		ID:        uuid.New(),
		UserID:    userID, // internal -- must not reach the wire
		Type:      notification.TypeOrderPlaced,
		Title:     "Order Placed",
		Body:      "Your order has been placed.",
		IsRead:    false,
		Data:      data,
		CreatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"id", "type", "title", "body", "is_read", "created_at"}, keysOf(fields),
		"the response must expose exactly these fields -- this key-set assertion is the real control against "+
			"Data leaking back in, since it is a []byte and would marshal to base64 rather than the plaintext "+
			"checked below")

	assert.NotContains(t, string(raw), userID.String(),
		"the caller is always the authenticated user; echoing user_id back adds nothing")
	// []byte marshals to base64, not plaintext, so a plaintext NotContains check
	// can never fire even if Data were re-added to the DTO. Assert against the
	// base64 encoding instead so this check is actually capable of catching
	// that regression.
	assert.NotContains(t, string(raw), base64.StdEncoding.EncodeToString(data),
		"Data is a raw job payload and must never pass through as raw bytes")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
