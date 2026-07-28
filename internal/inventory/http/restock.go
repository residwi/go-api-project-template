package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// restockRequest has no params.go counterpart: inventory.Service.Restock
// already takes a plain int, not a request struct, so there is no
// dto-in-the-core cycle to break here. The wire type's only job is to carry
// the validate tag.
type restockRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

func (h *handler) Restock(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[restockRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.service.Restock(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
