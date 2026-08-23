package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type WebhookProcessor interface {
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

type WebhookHandler struct {
	service WebhookProcessor
	logger  *slog.Logger
}

func NewWebhookHandler(service WebhookProcessor, log *slog.Logger) *WebhookHandler {
	return &WebhookHandler{service: service, logger: log}
}

const webhookSignatureHeader = "X-Webhook-Signature"

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "webhook: failed to read body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.service.HandleWebhook(r.Context(), body, r.Header.Get(webhookSignatureHeader)); err != nil {
		response.HandleErr(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
