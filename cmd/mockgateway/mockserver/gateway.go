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
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	gatewaymock "github.com/residwi/go-api-project-template/internal/features/payment/adapter/gateway/mock"
)

type Option func(*mockServer)

const statusSuccess = "success"

const webhookTimeout = 10 * time.Second

const mockSecretHeader = "X-Mock-Webhook-Secret" //nolint:gosec // G101: an HTTP header name, not a credential value

type chargeRecord struct {
	Response gatewaymock.ChargeResponse
	Metadata map[string]string
}

type mockServer struct {
	mu            sync.Mutex
	charges       map[string]chargeRecord
	refunds       map[string]gatewaymock.RefundResponse
	webhookSecret string
	logger        *slog.Logger
	transport     http.RoundTripper
}

func WithWebhookSecret(secret string) Option {
	return func(s *mockServer) { s.webhookSecret = secret }
}

func RegisterRoutes(mux *http.ServeMux, log *slog.Logger, opts ...Option) {
	s := &mockServer{
		charges: make(map[string]chargeRecord),
		refunds: make(map[string]gatewaymock.RefundResponse),
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
	var req gatewaymock.ChargeRequest
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
	var resp gatewaymock.ChargeResponse

	if req.PaymentMethodID != "" {
		if req.Amount%100 == 99 { //nolint:mnd // sentinel test value
			resp = gatewaymock.ChargeResponse{
				TransactionID: txnID,
				Status:        "failed",
			}
		} else {
			resp = gatewaymock.ChargeResponse{
				TransactionID: txnID,
				Status:        statusSuccess,
			}
		}
	} else {
		resp = gatewaymock.ChargeResponse{
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
	var req gatewaymock.RefundRequest
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

	resp := gatewaymock.RefundResponse{
		RefundID: uuid.New().String(),
		Status:   statusSuccess,
	}
	if req.IdempotencyKey != "" {
		s.refunds[req.IdempotencyKey] = resp
	}
	writeJSONResponse(w, resp)
}

func (s *mockServer) handleWebhookTrigger(w http.ResponseWriter, r *http.Request) {
	if s.webhookSecret == "" || !hmac.Equal([]byte(r.Header.Get(mockSecretHeader)), []byte(s.webhookSecret)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

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
	} else if err := validateWebhookURL(r.Context(), webhookURL); err != nil {
		s.logger.WarnContext(
			r.Context(),
			"webhook url rejected",
			slog.String("error", err.Error()),
			slog.String("webhook_url", webhookURL),
		)
		http.Error(w, "invalid webhook url", http.StatusBadRequest)
		return
	}

	client := s.webhookClient(triggerReq.WebhookURL != "")
	reqCtx := context.WithoutCancel(r.Context())

	go func() {
		ctx, cancel := context.WithTimeout(reqCtx, webhookTimeout)
		defer cancel()

		req, reqErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			webhookURL,
			bytes.NewReader(body),
		)
		if reqErr != nil {
			s.logger.ErrorContext(reqCtx, "webhook request creation failed", slog.String("error", reqErr.Error()))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if s.webhookSecret != "" {
			mac := hmac.New(sha256.New, []byte(s.webhookSecret))
			mac.Write(body)
			req.Header.Set("X-Webhook-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := client.Do(req)
		if err != nil {
			s.logger.ErrorContext(reqCtx, "webhook trigger failed", slog.String("error", err.Error()))
			return
		}
		_ = resp.Body.Close()
		s.logger.InfoContext(
			reqCtx,
			"webhook triggered",
			slog.Int("status", resp.StatusCode),
			slog.String("event", event),
		)
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "webhook_triggered"})
}

func (s *mockServer) webhookClient(external bool) *http.Client {
	transport := s.transport
	if transport == nil && external {
		transport = publicOnlyTransport()
	}

	return &http.Client{
		Timeout:   webhookTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return fmt.Errorf("webhook: refusing redirect to %s", req.URL.Redacted())
		},
	}
}

func withTransport(rt http.RoundTripper) Option {
	return func(s *mockServer) { s.transport = rt }
}

func validateWebhookURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse webhook url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook url scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook url has no host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if !isPublicUnicast(ip) {
			return fmt.Errorf("webhook url points at non-public address %s", ip)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve webhook host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("webhook host %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		if !isPublicUnicast(addr.IP) {
			return fmt.Errorf("webhook host %q resolves to non-public address %s", host, addr.IP)
		}
	}
	return nil
}

func isPublicUnicast(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublicPrefixes() {
		if prefix.Contains(addr) {
			return false
		}
	}

	return true
}

func nonPublicPrefixes() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
	}
}

func publicOnlyTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: webhookTimeout,
			Control: refuseNonPublicDial,
		}).DialContext,
		TLSHandshakeTimeout:   webhookTimeout,
		ResponseHeaderTimeout: webhookTimeout,
		DisableKeepAlives:     true,
	}
}

func refuseNonPublicDial(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("webhook: splitting dial address %q: %w", address, err)
	}
	if !isPublicUnicast(net.ParseIP(host)) {
		return fmt.Errorf("webhook: refusing dial to non-public address %s", host)
	}

	return nil
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
