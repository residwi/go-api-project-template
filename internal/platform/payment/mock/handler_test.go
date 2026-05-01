package mock_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/payment"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
)

func newMockMux() *http.ServeMux {
	mux := http.NewServeMux()
	mockgw.RegisterRoutes(mux)
	return mux
}

func TestHandleCharge(t *testing.T) {
	t.Run("direct charge success", func(t *testing.T) {
		mux := newMockMux()
		body := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-ok-1"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp payment.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "success", resp.Status)
		assert.NotEmpty(t, resp.TransactionID)
		assert.Empty(t, resp.PaymentURL)
	})

	t.Run("direct charge failure when amount ends in 99", func(t *testing.T) {
		mux := newMockMux()
		body := `{"amount":1099,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-fail-99"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp payment.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "failed", resp.Status)
	})

	t.Run("redirect flow when no payment method", func(t *testing.T) {
		mux := newMockMux()
		body := `{"amount":2000,"currency":"USD","idempotency_key":"charge-redirect-1"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp payment.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "pending", resp.Status)
		assert.NotEmpty(t, resp.PaymentURL)
	})

	t.Run("idempotency returns same response", func(t *testing.T) {
		mux := newMockMux()
		body := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-idemp-1"}`

		req1 := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req1.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()
		mux.ServeHTTP(w1, req1)

		var resp1 payment.ChargeResponse
		require.NoError(t, json.NewDecoder(w1.Body).Decode(&resp1))

		req2 := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		mux.ServeHTTP(w2, req2)

		var resp2 payment.ChargeResponse
		require.NoError(t, json.NewDecoder(w2.Body).Decode(&resp2))

		assert.Equal(t, resp1, resp2)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		mux := newMockMux()
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleRefund(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := newMockMux()
		body := `{"transaction_id":"txn_123","amount":500,"reason":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/refund", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp payment.RefundResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "success", resp.Status)
		assert.NotEmpty(t, resp.RefundID)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		mux := newMockMux()
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/refund", strings.NewReader("bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleWebhookTrigger(t *testing.T) {
	t.Run("triggers webhook for existing charge", func(t *testing.T) {
		mux := newMockMux()

		// First, create a charge
		chargeBody := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"webhook-test-1"}`
		chargeReq := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(chargeBody))
		chargeReq.Header.Set("Content-Type", "application/json")
		chargeW := httptest.NewRecorder()
		mux.ServeHTTP(chargeW, chargeReq)
		require.Equal(t, http.StatusOK, chargeW.Code)

		// Trigger webhook with a dummy URL (we don't need it to actually POST)
		triggerBody := `{"idempotency_key":"webhook-test-1","webhook_url":"http://127.0.0.1:1","event":"success"}`
		triggerReq := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(triggerBody))
		triggerReq.Header.Set("Content-Type", "application/json")
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)
	})

	t.Run("triggers webhook with default event", func(t *testing.T) {
		mux := newMockMux()

		chargeBody := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"webhook-test-2"}`
		chargeReq := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(chargeBody))
		chargeReq.Header.Set("Content-Type", "application/json")
		chargeW := httptest.NewRecorder()
		mux.ServeHTTP(chargeW, chargeReq)
		require.Equal(t, http.StatusOK, chargeW.Code)

		// No event field → defaults to "success"; no webhook_url → uses default
		triggerBody := `{"idempotency_key":"webhook-test-2","webhook_url":"http://127.0.0.1:1"}`
		triggerReq := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(triggerBody))
		triggerReq.Header.Set("Content-Type", "application/json")
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)
	})

	t.Run("uses default webhook URL when not provided", func(t *testing.T) {
		mux := newMockMux()

		chargeBody := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"webhook-test-default-url"}`
		chargeReq := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(chargeBody))
		chargeReq.Header.Set("Content-Type", "application/json")
		chargeW := httptest.NewRecorder()
		mux.ServeHTTP(chargeW, chargeReq)
		require.Equal(t, http.StatusOK, chargeW.Code)

		// No webhook_url field → defaults to localhost:8080
		triggerBody := `{"idempotency_key":"webhook-test-default-url","event":"success"}`
		triggerReq := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(triggerBody))
		triggerReq.Header.Set("Content-Type", "application/json")
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)
	})

	t.Run("webhook POST succeeds", func(t *testing.T) {
		mux := newMockMux()

		// Create a charge first
		chargeBody := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"webhook-success-1"}`
		chargeReq := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(chargeBody))
		chargeReq.Header.Set("Content-Type", "application/json")
		chargeW := httptest.NewRecorder()
		mux.ServeHTTP(chargeW, chargeReq)
		require.Equal(t, http.StatusOK, chargeW.Code)

		// Start a real server to receive the webhook POST
		called := make(chan struct{})
		webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			close(called)
		}))
		defer webhookServer.Close()

		// Trigger webhook pointing to our test server
		triggerBody := `{"idempotency_key":"webhook-success-1","webhook_url":"` + webhookServer.URL + `","event":"success"}`
		triggerReq := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(triggerBody))
		triggerReq.Header.Set("Content-Type", "application/json")
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)
		assert.Equal(t, http.StatusAccepted, triggerW.Code)

		// Wait for the goroutine to complete the POST
		<-called
	})

	t.Run("returns 404 for unknown idempotency key", func(t *testing.T) {
		mux := newMockMux()
		body := `{"idempotency_key":"nonexistent","webhook_url":"http://example.com/hook"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		mux := newMockMux()
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader("bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
