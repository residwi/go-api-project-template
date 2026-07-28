package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/shipping"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *shipping.Service
	validator *validator.Validator
}

func (h *adminHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[shipping.CreateShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.CreateShipment(r.Context(), orderID, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, shipment)
}

func (h *adminHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[shipping.UpdateTrackingRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.UpdateTracking(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, shipment)
}

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

	response.OK(w, shipment)
}
