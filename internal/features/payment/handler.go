package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

const webhookSignatureHeader = "X-Webhook-Signature"

type webhookHandler struct {
	service *Service
	secret  string
}

func (h *webhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		slog.ErrorContext(r.Context(), "webhook: failed to read body", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// When a secret is configured, require a valid HMAC signature so a third party
	// can't forge payment events (e.g. POST a fake "success" to fulfil an order).
	if h.secret != "" && !verifyWebhookSignature(h.secret, body, r.Header.Get(webhookSignatureHeader)) {
		slog.WarnContext(r.Context(), "webhook: invalid or missing signature")
		response.Unauthorized(w, "invalid webhook signature")
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.ErrorContext(r.Context(), "webhook: invalid payload", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.service.HandleWebhook(r.Context(), payload); err != nil {
		slog.ErrorContext(r.Context(), "webhook processing failed", "error", err)
		response.InternalError(w)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// verifyWebhookSignature checks a hex-encoded HMAC-SHA256 of the raw body using
// a constant-time comparison.
func verifyWebhookSignature(secret string, body []byte, provided string) bool {
	if provided == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}
