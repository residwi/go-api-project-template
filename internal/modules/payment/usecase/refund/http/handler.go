package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type UseCase interface {
	Execute(ctx context.Context, paymentID uuid.UUID) error
}

type Handler struct {
	cmd UseCase
}

func New(cmd UseCase) *Handler {
	return &Handler{cmd: cmd}
}

type refundResponse struct {
	Status string `json:"status"`
}

func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
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
