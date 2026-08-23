package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

// Signature verification and the business dispatch it guards used to need a
// REAL webhook.UseCase in this package, because that check lived inside a
// private-mocked slice's Execute. Now that HandleWebhook is a payment.Service
// method, TestService_HandleWebhook_SignatureVerification proves the check
// directly with no HTTP involved, so this file goes back to the standard
// mocked-port shape every other slice's handler test already uses.

func TestHandler_HandleWebhook(t *testing.T) {
	t.Parallel()

	t.Run("success threads body and signature header to the service", func(t *testing.T) {
		t.Parallel()

		mux, service := setupWebhookMux(t)

		body, err := json.Marshal(map[string]any{
			"event":    "success",
			"metadata": map[string]any{"payment_id": uuid.New().String()},
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
		service.EXPECT().HandleWebhook(mock.Anything, body, "").Return(apperror.ErrUnauthorized)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func setupWebhookMux(t *testing.T) (*http.ServeMux, *MockWebhookProcessor) {
	service := NewMockWebhookProcessor(t)

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	api.HandleFunc("POST /payments/webhook", NewHandler(service, testhelper.DiscardLogger()).HandleWebhook)

	return mux, service
}
