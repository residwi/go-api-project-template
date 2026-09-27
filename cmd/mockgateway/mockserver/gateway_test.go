package mockserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gatewaymock "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/mock"
)

const publicWebhookURL = "http://203.0.113.10/hook"

const testWebhookSecret = "test-webhook-secret"

func TestHandleCharge(t *testing.T) {
	t.Parallel()

	t.Run("direct charge success", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-ok-1"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp gatewaymock.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "success", resp.Status)
		assert.NotEmpty(t, resp.TransactionID)
		assert.Empty(t, resp.PaymentURL)
	})

	t.Run("direct charge failure when amount ends in 99", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"amount":1099,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-fail-99"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp gatewaymock.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "failed", resp.Status)
	})

	t.Run("redirect flow when no payment method", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"amount":2000,"currency":"USD","idempotency_key":"charge-redirect-1"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp gatewaymock.ChargeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "pending", resp.Status)
		assert.NotEmpty(t, resp.PaymentURL)
	})

	t.Run("idempotency returns same response", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"charge-idemp-1"}`

		req1 := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req1.Header.Set("Content-Type", "application/json")
		w1 := httptest.NewRecorder()
		mux.ServeHTTP(w1, req1)

		var resp1 gatewaymock.ChargeResponse
		require.NoError(t, json.NewDecoder(w1.Body).Decode(&resp1))

		req2 := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		mux.ServeHTTP(w2, req2)

		var resp2 gatewaymock.ChargeResponse
		require.NoError(t, json.NewDecoder(w2.Body).Decode(&resp2))

		assert.Equal(t, resp1, resp2)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleRefund(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"transaction_id":"txn_123","amount":500,"reason":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/refund", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp gatewaymock.RefundResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "success", resp.Status)
		assert.NotEmpty(t, resp.RefundID)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		req := httptest.NewRequest(http.MethodPost, "/mock/payment/refund", strings.NewReader("bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleWebhookTrigger(t *testing.T) {
	t.Parallel()

	t.Run("triggers webhook for existing charge", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-test-1")

		triggerBody := `{"idempotency_key":"webhook-test-1","webhook_url":"` + publicWebhookURL + `","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)

		got := rt.awaitCall(t)
		assert.Contains(t, got, "203.0.113.10")
	})

	t.Run("triggers webhook with default event", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-test-2")

		// No event field defaults to "success".
		triggerBody := `{"idempotency_key":"webhook-test-2","webhook_url":"` + publicWebhookURL + `"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)
		rt.awaitCall(t)
	})

	t.Run("uses default webhook URL when not provided", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-test-default-url")

		// No webhook_url field → defaults to localhost:8080, which is trusted
		// and delivered without SSRF validation.
		triggerBody := `{"idempotency_key":"webhook-test-default-url","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusAccepted, triggerW.Code)
		got := rt.awaitCall(t)
		assert.Contains(t, got, "localhost:8080")
	})

	t.Run("delivers to a public webhook URL", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-success-1")

		triggerBody := `{"idempotency_key":"webhook-success-1","webhook_url":"` + publicWebhookURL + `","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)
		assert.Equal(t, http.StatusAccepted, triggerW.Code)

		got := rt.awaitCall(t)
		assert.Contains(t, got, "203.0.113.10")
	})

	t.Run("does not follow a redirect to an internal target", func(t *testing.T) {
		t.Parallel()

		// An allowed public URL whose response 307s to a loopback target must
		// not deliver the signed webhook to that internal host.
		rt := newRecordingTransport()
		rt.redirectTo = "http://127.0.0.1:9/latest/meta-data/"
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-redirect-1")

		triggerBody := `{"idempotency_key":"webhook-redirect-1","webhook_url":"` + publicWebhookURL + `","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)
		assert.Equal(t, http.StatusAccepted, triggerW.Code)

		got := rt.awaitCall(t)
		assert.Contains(t, got, "203.0.113.10")
		rt.assertNoMoreCalls(t)

		recorded := rt.recorded()
		require.Len(t, recorded, 1)
		assert.NotContains(t, recorded[0], "127.0.0.1")
	})

	t.Run("rejects a loopback webhook URL", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-loopback-1")

		triggerBody := `{"idempotency_key":"webhook-loopback-1","webhook_url":"http://127.0.0.1:9/","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusBadRequest, triggerW.Code)
		rt.assertNoMoreCalls(t)
	})

	t.Run("rejects a private webhook URL", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-private-1")

		triggerBody := `{"idempotency_key":"webhook-private-1","webhook_url":"http://10.0.0.5/hook","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusBadRequest, triggerW.Code)
		rt.assertNoMoreCalls(t)
	})

	t.Run("rejects the link-local metadata endpoint", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-metadata-1")

		triggerBody := `{"idempotency_key":"webhook-metadata-1","webhook_url":"http://169.254.169.254/latest/meta-data/","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusBadRequest, triggerW.Code)
		rt.assertNoMoreCalls(t)
	})

	t.Run("rejects a non-http scheme", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "webhook-scheme-1")

		triggerBody := `{"idempotency_key":"webhook-scheme-1","webhook_url":"file:///etc/passwd","event":"success"}`
		triggerReq := newTriggerRequest(triggerBody)
		triggerW := httptest.NewRecorder()
		mux.ServeHTTP(triggerW, triggerReq)

		assert.Equal(t, http.StatusBadRequest, triggerW.Code)
		rt.assertNoMoreCalls(t)
	})

	t.Run("returns 404 for unknown idempotency key", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		body := `{"idempotency_key":"nonexistent","webhook_url":"` + publicWebhookURL + `"}`
		req := newTriggerRequest(body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		t.Parallel()

		mux := newMockMux()
		req := newTriggerRequest("bad")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleWebhookTriggerRequiresSecret(t *testing.T) {
	t.Parallel()

	t.Run("rejects a request without the secret header", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "gate-missing")

		req := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger",
			strings.NewReader(`{"idempotency_key":"gate-missing","event":"success"}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, rt.recorded())
	})

	t.Run("rejects a wrong secret", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := newMockMuxWithTransport(rt)
		mustCharge(t, mux, "gate-wrong")

		req := newTriggerRequest(`{"idempotency_key":"gate-wrong","event":"success"}`)
		req.Header.Set(mockSecretHeader, "not-the-secret")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, rt.recorded())
	})

	t.Run("rejects every trigger when no secret is configured", func(t *testing.T) {
		t.Parallel()

		rt := newRecordingTransport()
		mux := http.NewServeMux()
		RegisterRoutes(mux, slog.New(slog.DiscardHandler), withTransport(rt))
		mustCharge(t, mux, "gate-unconfigured")

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, newTriggerRequest(`{"idempotency_key":"gate-unconfigured","event":"success"}`))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, rt.recorded())
	})
}

func TestPublicOnlyTransport(t *testing.T) {
	t.Parallel()

	dial := func(t *testing.T, target string) error {
		t.Helper()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		require.NoError(t, err)
		resp, err := (&http.Client{Transport: publicOnlyTransport()}).Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}

		return err
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)

	t.Run("refuses a literal loopback address at connect time", func(t *testing.T) {
		t.Parallel()

		err := dial(t, srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing dial to non-public address")
	})

	t.Run("refuses a hostname once it resolves to loopback", func(t *testing.T) {
		t.Parallel()

		err := dial(t, "http://localhost:"+port)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing dial to non-public address")
	})
}

func TestIsPublicUnicast(t *testing.T) {
	t.Parallel()

	rejects := func(t *testing.T, addrs ...string) {
		t.Helper()
		for _, a := range addrs {
			assert.False(t, isPublicUnicast(net.ParseIP(a)), a)
		}
	}

	t.Run("rejects non-public IPv4", func(t *testing.T) {
		t.Parallel()
		rejects(t, "127.0.0.1", "0.0.0.0", "0.0.0.1", "169.254.169.254", "10.0.0.1", "100.64.0.1")
	})

	t.Run("rejects non-public IPv6", func(t *testing.T) {
		t.Parallel()
		rejects(t, "::1", "fe80::1", "fc00::1")
	})

	t.Run("rejects IPv4 smuggled inside IPv6", func(t *testing.T) {
		t.Parallel()
		rejects(t, "::ffff:127.0.0.1", "::127.0.0.1", "64:ff9b::7f00:1")
	})

	t.Run("accepts public addresses", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isPublicUnicast(net.ParseIP("93.184.216.34")))
		assert.True(t, isPublicUnicast(net.ParseIP("2606:4700::1111")))
	})
}

type recordingTransport struct {
	mu         sync.Mutex
	requests   []string
	calls      chan string
	redirectTo string
}

func newRecordingTransport() *recordingTransport {
	return &recordingTransport{calls: make(chan string, 8)}
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.requests = append(rt.requests, req.URL.String())
	rt.mu.Unlock()

	select {
	case rt.calls <- req.URL.String():
	default:
	}

	status := http.StatusOK
	header := make(http.Header)
	if rt.redirectTo != "" {
		status = http.StatusTemporaryRedirect
		header.Set("Location", rt.redirectTo)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func (rt *recordingTransport) awaitCall(t *testing.T) string {
	t.Helper()
	select {
	case got := <-rt.calls:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("expected an outbound webhook request")
		return ""
	}
}

func (rt *recordingTransport) assertNoMoreCalls(t *testing.T) {
	t.Helper()
	select {
	case got := <-rt.calls:
		t.Fatalf("unexpected outbound webhook request to %s", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func (rt *recordingTransport) recorded() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.requests...)
}

func newMockMux() *http.ServeMux {
	mux := http.NewServeMux()
	RegisterRoutes(mux, slog.New(slog.DiscardHandler), WithWebhookSecret(testWebhookSecret))
	return mux
}

func newMockMuxWithTransport(rt http.RoundTripper) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterRoutes(mux, slog.New(slog.DiscardHandler), WithWebhookSecret(testWebhookSecret), withTransport(rt))
	return mux
}

func mustCharge(t *testing.T, mux *http.ServeMux, key string) {
	t.Helper()
	body := `{"amount":1000,"currency":"USD","payment_method_id":"pm_test","idempotency_key":"` + key + `"}`
	req := httptest.NewRequest(http.MethodPost, "/mock/payment/charge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func newTriggerRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/mock/payment/webhook/trigger", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mockSecretHeader, testWebhookSecret)

	return req
}
