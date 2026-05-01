package notification_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/middleware"
	notifMocks "github.com/residwi/go-api-project-template/mocks/notification"
)

func setupNotificationMux(t *testing.T) (*http.ServeMux, *notifMocks.MockRepository, middleware.UserContext) {
	repo := notifMocks.NewMockRepository(t)
	svc := notification.NewService(repo)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	notification.RegisterRoutes(authed, notification.RouteDeps{
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
		repo.EXPECT().MarkRead(mock.Anything, id).Return(nil)

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
		repo.EXPECT().MarkRead(mock.Anything, id).Return(core.ErrNotFound)

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
