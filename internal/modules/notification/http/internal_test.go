package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func newTestHandler() *handler {
	return &handler{
		service: &notification.Service{},
	}
}

func TestHandler_List(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/notifications", nil)
		w := httptest.NewRecorder()

		h.List(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})
}

func TestHandler_MarkRead(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/notifications/"+uuid.NewString()+"/read", nil)
		w := httptest.NewRecorder()

		h.MarkRead(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/notifications/bad/read", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{UserID: uuid.New(), Role: "user"})
		r = r.WithContext(ctx)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.MarkRead(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})
}

func TestHandler_MarkAllRead(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/notifications/read-all", nil)
		w := httptest.NewRecorder()

		h.MarkAllRead(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestHandler_UnreadCount(t *testing.T) {
	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
		w := httptest.NewRecorder()

		h.UnreadCount(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

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
