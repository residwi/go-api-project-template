package http

import (
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_List_Success(t *testing.T) {
	t.Parallel()

	t.Run("success with notifications", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		now := time.Now()
		notifications := []domain.Notification{
			{
				ID:        uuid.New(),
				UserID:    uc.UserID,
				Type:      domain.TypeOrderPlaced,
				Title:     "Order Placed",
				CreatedAt: now,
			},
		}
		reader.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}

func TestHandler_List_ReaderError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		reader.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_UnreadCount_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		reader.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(3, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(3), data["count"], 0.001)
	})
}

func TestHandler_UnreadCount_ReaderError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		reader.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_List_Pagination(t *testing.T) {
	t.Parallel()

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, reader, uc := setupQueryMux(t)

		now := time.Now()
		notifications := make([]domain.Notification, 21)
		for i := range notifications {
			notifications[i] = domain.Notification{
				ID:        uuid.New(),
				UserID:    uc.UserID,
				Type:      domain.TypeOrderPlaced,
				Title:     "Order",
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		reader.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, pagination["has_more"])
		assert.NotEmpty(t, pagination["next_cursor"])
	})
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

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

func TestHandler_UnreadCount(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/notifications/unread-count", nil)
		w := httptest.NewRecorder()

		h.UnreadCount(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestToNotificationResponse_OmitsUserIDAndRawPayload(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data := []byte(`{"order_id":"distinguishable-raw-payload"}`)

	got := toNotificationResponse(domain.Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Type:      domain.TypeOrderPlaced,
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
	assert.ElementsMatch(
		t,
		[]string{"id", "type", "title", "body", "is_read", "created_at"},
		slices.Collect(maps.Keys(fields)),
		"the response must expose exactly these fields -- this key-set assertion is the real control against "+
			"Data leaking back in, since it is a []byte and would marshal to base64 rather than the plaintext "+
			"checked below",
	)

	assert.NotContains(t, string(raw), userID.String(),
		"the caller is always the authenticated user; echoing user_id back adds nothing")
	// []byte marshals to base64, so a plaintext NotContains could never fire even
	// if Data came back. Assert the base64 form.
	assert.NotContains(t, string(raw), base64.StdEncoding.EncodeToString(data),
		"Data is a raw job payload and must never pass through as raw bytes")
}

func setupQueryMux(t *testing.T) (*http.ServeMux, *MockNotificationReader, middleware.UserContext) {
	reader := NewMockNotificationReader(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	h := New(reader)
	authed.HandleFunc("GET /notifications", h.List)
	authed.HandleFunc("GET /notifications/unread-count", h.UnreadCount)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, reader, uc
}

func notifAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}
