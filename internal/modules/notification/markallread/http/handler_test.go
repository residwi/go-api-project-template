package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func TestHandler_MarkAllRead_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd, uc := setupMarkAllReadMux(t)

		cmd.EXPECT().Execute(mock.Anything, uc.UserID).Return(nil)

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

		mux, cmd, uc := setupMarkAllReadMux(t)

		cmd.EXPECT().Execute(mock.Anything, uc.UserID).Return(assert.AnError)

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

func setupMarkAllReadMux(t *testing.T) (*http.ServeMux, *MockAllNotificationsMarker, middleware.UserContext) {
	cmd := NewMockAllNotificationsMarker(t)

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("PUT /notifications/read-all", New(cmd).MarkAllRead)

	uc := middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	}

	return mux, cmd, uc
}

func notifAuth(r *http.Request, uc middleware.UserContext) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}
