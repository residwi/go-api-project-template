package mockserver

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

	"github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway"
)

type Option func(*mockServer)

const statusSuccess = "success"

type chargeRecord struct {
	Response gateway.ChargeResponse
	Metadata map[string]string
}

type mockServer struct {
	mu            sync.Mutex
	charges       map[string]chargeRecord
	refunds       map[string]gateway.RefundResponse
	webhookSecret string
	logger        *slog.Logger
}

func WithWebhookSecret(secret string) Option {
	return func(s *mockServer) { s.webhookSecret = secret }
}

func RegisterRoutes(mux *http.ServeMux, log *slog.Logger, opts ...Option) {
	s := &mockServer{
		charges: make(map[string]chargeRecord),
		refunds: make(map[string]gateway.RefundResponse),
		logger:  log,
	}
	for _, opt := range opts {
		opt(s)
	}
	mux.HandleFunc("POST /mock/payment/charge", s.handleCharge)
	mux.HandleFunc("POST /mock/payment/refund", s.handleRefund)
	mux.HandleFunc("POST /mock/payment/webhook/trigger", s.handleWebhookTrigger)
}

func (s *mockServer) handleCharge(w http.ResponseWriter, r *http.Request) {
	var req gateway.ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.charges[req.IdempotencyKey]; ok {
		writeJSONResponse(w, existing.Response)
		return
	}

	txnID := uuid.New().String()
	var resp gateway.ChargeResponse

	if req.PaymentMethodID != "" {
		if req.Amount%100 == 99 { //nolint:mnd // sentinel test value
			resp = gateway.ChargeResponse{
				TransactionID: txnID,
				Status:        "failed",
			}
		} else {
			resp = gateway.ChargeResponse{
				TransactionID: txnID,
				Status:        statusSuccess,
			}
		}
	} else {
		resp = gateway.ChargeResponse{
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
	var req gateway.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if req.IdempotencyKey != "" {
		if existing, ok := s.refunds[req.IdempotencyKey]; ok {
			writeJSONResponse(w, existing)
			return
		}
	}

	resp := gateway.RefundResponse{
		RefundID: uuid.New().String(),
		Status:   statusSuccess,
	}
	if req.IdempotencyKey != "" {
		s.refunds[req.IdempotencyKey] = resp
	}
	writeJSONResponse(w, resp)
}

func (s *mockServer) handleWebhookTrigger(w http.ResponseWriter, r *http.Request) {
	var triggerReq struct {
		IdempotencyKey string `json:"idempotency_key"`
		WebhookURL     string `json:"webhook_url"`
		Event          string `json:"event"`
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
		req, reqErr := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			webhookURL,
			bytes.NewReader(body),
		)
		if reqErr != nil {
			s.logger.ErrorContext(r.Context(), "webhook request creation failed", slog.String("error", reqErr.Error()))
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
			s.logger.ErrorContext(r.Context(), "webhook trigger failed", slog.String("error", err.Error()))
			return
		}
		_ = resp.Body.Close()
		s.logger.InfoContext(
			r.Context(),
			"webhook triggered",
			slog.Int("status", resp.StatusCode),
			slog.String("event", event),
		)
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "webhook_triggered"})
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
