package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func (h *adminHandler) MarkDelivered(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.service.MarkDelivered(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
