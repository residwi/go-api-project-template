package http

import (
	"bytes"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	t.Parallel()

	t.Run("success threads body and signature header to the service", func(t *testing.T) {
		t.Parallel()

		mux, service := setupWebhookMux(t)

		body, err := json.Marshal(map[string]any{
			"event":    "success",
			"metadata": map[string]any{"payment_id": uuid.Must(uuid.NewV7()).String()},
		})
		require.NoError(t, err)

		service.EXPECT().HandleWebhook(mock.Anything, body, "sig-abc").Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("X-Webhook-Signature", "sig-abc")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("service error is mapped through response.HandleErr", func(t *testing.T) {
		t.Parallel()

		mux, service := setupWebhookMux(t)

		body := []byte(`{"event":"success"}`)
		service.EXPECT().HandleWebhook(mock.Anything, body, "").Return(errs.ErrUnauthorized)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func setupWebhookMux(t *testing.T) (*http.ServeMux, *MockWebhookProcessor) {
	service := NewMockWebhookProcessor(t)

	mux := http.NewServeMux()
	api := web.NewRouter(mux, testutil.DiscardLogger()).Group("/api")
	api.HandleFunc("POST /payments/webhook", NewWebhookHandler(service, testutil.DiscardLogger()).HandleWebhook)

	return mux, service
}
