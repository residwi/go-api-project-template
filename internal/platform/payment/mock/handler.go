package mock

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/payment"
)

const statusSuccess = "success"

type chargeRecord struct {
	Response payment.ChargeResponse
	Metadata map[string]string
}

type mockServer struct {
	mu            sync.Mutex
	charges       map[string]chargeRecord // idempotency_key -> record
	webhookSecret string
}

// Option configures the mock payment server.
type Option func(*mockServer)

// WithWebhookSecret makes the mock sign triggered webhooks with the same
// HMAC-SHA256 scheme the real webhook handler verifies.
func WithWebhookSecret(secret string) Option {
	return func(s *mockServer) { s.webhookSecret = secret }
}

func RegisterRoutes(mux *http.ServeMux, opts ...Option) {
	s := &mockServer{
		charges: make(map[string]chargeRecord),
	}
	for _, opt := range opts {
		opt(s)
	}
	mux.HandleFunc("POST /mock/payment/charge", s.handleCharge)
	mux.HandleFunc("POST /mock/payment/refund", s.handleRefund)
	mux.HandleFunc("POST /mock/payment/webhook/trigger", s.handleWebhookTrigger)
}

func (s *mockServer) handleCharge(w http.ResponseWriter, r *http.Request) {
	var req payment.ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check
	if existing, ok := s.charges[req.IdempotencyKey]; ok {
		writeJSONResponse(w, existing.Response)
		return
	}

	txnID := uuid.New().String()
	var resp payment.ChargeResponse

	if req.PaymentMethodID != "" {
		// Direct charge
		if req.Amount%100 == 99 { //nolint:mnd // sentinel test value
			resp = payment.ChargeResponse{
				TransactionID: txnID,
				Status:        "failed",
			}
		} else {
			resp = payment.ChargeResponse{
				TransactionID: txnID,
				Status:        statusSuccess,
			}
		}
	} else {
		// Redirect flow
		resp = payment.ChargeResponse{
			TransactionID: txnID,
			Status:        "pending",
			PaymentURL:    fmt.Sprintf("http://localhost:8080/mock/payment/checkout/%s", txnID),
		}
	}

	s.charges[req.IdempotencyKey] = chargeRecord{
		Response: resp,
		Metadata: req.Metadata,
	}

	writeJSONResponse(w, resp)
}

func (s *mockServer) handleRefund(w http.ResponseWriter, r *http.Request) {
	var req payment.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := payment.RefundResponse{
		RefundID: uuid.New().String(),
		Status:   statusSuccess,
	}
	writeJSONResponse(w, resp)
}

func (s *mockServer) handleWebhookTrigger(w http.ResponseWriter, r *http.Request) {
	var triggerReq struct {
		IdempotencyKey string `json:"idempotency_key"`
		WebhookURL     string `json:"webhook_url"`
		Event          string `json:"event"` // success, failed
	}
	if err := json.NewDecoder(r.Body).Decode(&triggerReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	record, ok := s.charges[triggerReq.IdempotencyKey]
	s.mu.Unlock()

	if !ok {
		http.Error(w, "charge not found for idempotency key", http.StatusNotFound)
		return
	}

	event := triggerReq.Event
	if event == "" {
		event = statusSuccess
	}

	webhookPayload := map[string]any{
		"event":          event,
		"transaction_id": record.Response.TransactionID,
		"metadata":       record.Metadata,
	}

	body, _ := json.Marshal(webhookPayload)
	webhookURL := triggerReq.WebhookURL
	if webhookURL == "" {
		webhookURL = "http://localhost:8080/api/payments/webhook"
	}

	go func() {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodPost, webhookURL, bytes.NewReader(body))
		if reqErr != nil {
			slog.ErrorContext(r.Context(), "webhook request creation failed", "error", reqErr)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if s.webhookSecret != "" {
			mac := hmac.New(sha256.New, []byte(s.webhookSecret))
			mac.Write(body)
			req.Header.Set("X-Webhook-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.ErrorContext(r.Context(), "webhook trigger failed", "error", err)
			return
		}
		_ = resp.Body.Close()
		slog.InfoContext(r.Context(), "webhook triggered", "status", resp.StatusCode, "event", event)
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "webhook_triggered"})
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
