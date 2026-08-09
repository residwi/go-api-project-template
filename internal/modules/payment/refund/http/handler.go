package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// Command is what Handler needs from refund.Command: refund.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in
// handler_test.go.
type Command interface {
	Execute(ctx context.Context, paymentID uuid.UUID) error
}

type Handler struct {
	cmd Command
}

func New(cmd Command) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("POST /payments/{id}/refund", h.refund)
}

type refundResponse struct {
	Status string `json:"status"`
}

func (h *Handler) refund(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.cmd.Execute(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, refundResponse{Status: "refund_enqueued"})
}
