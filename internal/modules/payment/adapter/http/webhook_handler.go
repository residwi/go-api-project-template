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

type Handler struct {
	service WebhookProcessor
	logger  *slog.Logger
}

func NewHandler(service WebhookProcessor, log *slog.Logger) *Handler {
	return &Handler{service: service, logger: log}
}

const webhookSignatureHeader = "X-Webhook-Signature"

func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
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
