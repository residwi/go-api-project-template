package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
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

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "webhook: failed to read body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return nil
	}

	if err := h.service.HandleWebhook(r.Context(), body, r.Header.Get(webhookSignatureHeader)); err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	return nil
}
