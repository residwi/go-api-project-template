package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// refundResponse replaces the pre-refactor inline map[string]string literal
// with a named type -- same wire shape, now typed. Refund has no request
// params.go counterpart: payment.Service.Refund already takes a plain
// uuid.UUID, not a request struct (a partial-amount/reasoned refund isn't
// implemented today, so there is nothing to bind from a body).
type refundResponse struct {
	Status string `json:"status"`
}

func (h *adminHandler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Refund(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, refundResponse{Status: "refund_enqueued"})
}
