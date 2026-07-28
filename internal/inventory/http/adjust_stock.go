package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adjustRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}

func (h *handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[adjustRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.service.AdjustStock(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
