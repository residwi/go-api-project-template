package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Refunder interface {
	Execute(ctx context.Context, paymentID uuid.UUID) error
}

type Handler struct {
	usecase Refunder
}

func New(usecase Refunder) *Handler {
	return &Handler{usecase: usecase}
}

type refundResponse struct {
	Status string `json:"status"`
}

func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.usecase.Execute(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, refundResponse{Status: "refund_enqueued"})
}
