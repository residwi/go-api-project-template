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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	notifMocks "github.com/residwi/go-api-project-template/mocks/notification"
)

func TestHandler_List_Success(t *testing.T) {
	t.Run("success with notifications", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		now := time.Now()
		notifications := []notification.Notification{
			{ID: uuid.New(), UserID: uc.UserID, Type: notification.TypeOrderPlaced, Title: "Order Placed", CreatedAt: now},
		}
		repo.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

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

func TestHandler_List_ServiceError(t *testing.T) {
	t.Run("repo error", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		repo.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_MarkRead_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		id := uuid.New()
		repo.EXPECT().MarkRead(mock.Anything, uc.UserID, id).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+id.String()+"/read", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_MarkRead_ServiceError(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		id := uuid.New()
		repo.EXPECT().MarkRead(mock.Anything, uc.UserID, id).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+id.String()+"/read", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_MarkRead_InvalidUUID(t *testing.T) {
	t.Run("invalid UUID via mux", func(t *testing.T) {
		mux, _, uc := setupNotificationMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/not-a-uuid/read", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})
}

func TestHandler_MarkAllRead_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		repo.EXPECT().MarkAllRead(mock.Anything, uc.UserID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_MarkAllRead_ServiceError(t *testing.T) {
	t.Run("repo error", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		repo.EXPECT().MarkAllRead(mock.Anything, uc.UserID).Return(assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_UnreadCount_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		repo.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(3, nil)

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

func TestHandler_UnreadCount_ServiceError(t *testing.T) {
	t.Run("repo error", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		repo.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_List_Pagination(t *testing.T) {
	t.Run("has more results triggers cursor", func(t *testing.T) {
		mux, repo, uc := setupNotificationMux(t)

		now := time.Now()
		notifications := make([]notification.Notification, 21)
		for i := range notifications {
			notifications[i] = notification.Notification{
				ID:        uuid.New(),
				UserID:    uc.UserID,
				Type:      notification.TypeOrderPlaced,
				Title:     "Order",
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		repo.EXPECT().ListByUser(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

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

func TestToNotificationResponse_OmitsUserIDAndRawPayload(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data := []byte(`{"order_id":"distinguishable-raw-payload"}`)

	got := toNotificationResponse(notification.Notification{
		ID:        uuid.New(),
		UserID:    userID,
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
	assert.ElementsMatch(t, []string{"id", "type", "title", "body", "is_read", "created_at"}, slices.Collect(maps.Keys(fields)),
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

func newTestHandler() *handler {
	return &handler{
		service: &notification.Service{},
	}
}

func setupNotificationMux(t *testing.T) (*http.ServeMux, *notifMocks.MockRepository, middleware.UserContext) {
	repo := notifMocks.NewMockRepository(t)
	svc := notification.NewService(repo)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	RegisterRoutes(authed, RouteDeps{
		Service: svc,
	})

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, repo, uc
}

func notifAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}
