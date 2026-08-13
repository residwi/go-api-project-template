package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Command interface {
	Execute(ctx context.Context, payload []byte, signature string) error
}

type Handler struct {
	cmd    Command
	logger *slog.Logger
}

func New(cmd Command, log *slog.Logger) *Handler {
	return &Handler{cmd: cmd, logger: log}
}

func (h *Handler) RegisterHTTP(api *middleware.RouteGroup) {
	api.HandleFunc("POST /payments/webhook", h.handleWebhook)
}

const webhookSignatureHeader = "X-Webhook-Signature"

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.logger.ErrorContext(r.Context(), "webhook: failed to read body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.cmd.Execute(r.Context(), body, r.Header.Get(webhookSignatureHeader)); err != nil {
		response.HandleErr(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
