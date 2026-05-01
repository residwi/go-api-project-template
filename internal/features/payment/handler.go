package payment

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

type webhookHandler struct {
	service *Service
}

func (h *webhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
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
