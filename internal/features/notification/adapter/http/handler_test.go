package http

import (
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

	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_List_Success(t *testing.T) {
	t.Parallel()

	t.Run("success with notifications", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		now := time.Now()
		notifications := []domain.Notification{
			{
				ID:        uuid.New(),
				UserID:    uc.UserID,
				Title:     "Order Placed",
				CreatedAt: now,
			},
		}
		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

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

		mux, service, uc := setupMux(t)

		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(nil, assert.AnError)

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

		mux, service, uc := setupMux(t)

		service.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(3, nil)

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

		mux, service, uc := setupMux(t)

		service.EXPECT().CountUnread(mock.Anything, uc.UserID).Return(0, assert.AnError)

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

		mux, service, uc := setupMux(t)

		now := time.Now()
		notifications := make([]domain.Notification, 21)
		for i := range notifications {
			notifications[i] = domain.Notification{
				ID:        uuid.New(),
				UserID:    uc.UserID,
				Title:     "Order",
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
			}
		}
		service.EXPECT().List(mock.Anything, uc.UserID, mock.Anything).Return(notifications, nil)

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

func TestToNotificationResponse_OmitsUserID(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toNotificationResponse(domain.Notification{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     "Order Placed",
		Body:      "Your order has been placed.",
		IsRead:    false,
		CreatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(
		t,
		[]string{"id", "title", "body", "is_read", "created_at"},
		slices.Collect(maps.Keys(fields)),
	)

	assert.NotContains(t, string(raw), userID.String(),
		"the caller is always the authenticated user; echoing user_id back adds nothing")
}

func TestHandler_MarkRead_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		id := uuid.New()
		service.EXPECT().MarkRead(mock.Anything, uc.UserID, id).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+id.String()+"/read", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_MarkRead_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		id := uuid.New()
		service.EXPECT().MarkRead(mock.Anything, uc.UserID, id).Return(errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+id.String()+"/read", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_MarkRead_InvalidUUID(t *testing.T) {
	t.Parallel()

	t.Run("invalid UUID via mux", func(t *testing.T) {
		t.Parallel()

		mux, _, uc := setupMux(t)

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

func TestHandler_MarkRead(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/notifications/"+uuid.NewString()+"/read", nil)
		w := httptest.NewRecorder()

		h.MarkRead(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/notifications/bad/read", nil)
		ctx := middleware.SetIdentity(r.Context(), identity.Identity{UserID: uuid.New(), Role: "user"})
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

func TestHandler_MarkAllRead_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		service.EXPECT().MarkAllRead(mock.Anything, uc.UserID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_MarkAllRead_CommandError(t *testing.T) {
	t.Parallel()

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		mux, service, uc := setupMux(t)

		service.EXPECT().MarkAllRead(mock.Anything, uc.UserID).Return(assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil)
		r = notifAuth(r, uc)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_MarkAllRead(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodPut, "/notifications/read-all", nil)
		w := httptest.NewRecorder()

		h.MarkAllRead(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func setupMux(t *testing.T) (*http.ServeMux, *MockNotificationManager, identity.Identity) {
	service := NewMockNotificationManager(t)

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	h := NewHandler(service)
	authed.HandleFunc("GET /notifications", h.List)
	authed.HandleFunc("GET /notifications/unread-count", h.UnreadCount)
	authed.HandleFunc("PUT /notifications/{id}/read", h.MarkRead)
	authed.HandleFunc("PUT /notifications/read-all", h.MarkAllRead)

	uc := identity.Identity{
		UserID: uuid.New(),
		Role:   "user",
	}

	return mux, service, uc
}

func notifAuth(r *http.Request, uc identity.Identity) *http.Request {
	ctx := middleware.SetIdentity(r.Context(), uc)
	return r.WithContext(ctx)
}
