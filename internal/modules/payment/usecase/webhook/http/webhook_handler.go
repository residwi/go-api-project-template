package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type WebhookProcessor interface {
	Execute(ctx context.Context, payload []byte, signature string) error
}

type Handler struct {
	usecase WebhookProcessor
	logger  *slog.Logger
}

func New(usecase WebhookProcessor, log *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: log}
}

const webhookSignatureHeader = "X-Webhook-Signature"

func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "webhook: failed to read body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.usecase.Execute(r.Context(), body, r.Header.Get(webhookSignatureHeader)); err != nil {
		response.HandleErr(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
