package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/shipping"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type createShipmentRequest struct {
	Carrier        string `json:"carrier" validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (r createShipmentRequest) toCreateParams() shipping.CreateParams {
	return shipping.CreateParams{
		Carrier:        r.Carrier,
		TrackingNumber: r.TrackingNumber,
	}
}

func (h *adminHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.CreateShipment(r.Context(), orderID, req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toShipmentResponse(shipment))
}
