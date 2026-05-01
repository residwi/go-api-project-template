package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler() *handler {
	return &handler{
		service: &Service{},
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

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/notifications/bad/read", nil)
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
